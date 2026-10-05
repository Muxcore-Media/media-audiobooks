package internal

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	abv1 "github.com/Muxcore-Media/media-audiobooks/proto/gen/muxcore/audiobooks/v1"
)

// clampInt32 converts n to int32, saturating at the int32 bounds.
func clampInt32(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	if n < math.MinInt32 {
		return math.MinInt32
	}
	return int32(n) //nolint:gosec // bounds checked above
}

type Module struct {
	lis     net.Listener
	store   *Store
	grpcSrv *grpc.Server
	httpSrv *http.Server

	id         string
	grpcAddr   string
	httpAddr   string
	dataDir    string
	libraryDir string
	imageDir   string

	cfgMu sync.RWMutex
}

type Config struct {
	ID         string
	DataDir    string
	LibraryDir string
	GRPCAddr   string
	HTTPAddr   string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "media-audiobooks"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9670"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":9671"
	}
	if v := os.Getenv("AUDIOBOOKS_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("AUDIOBOOKS_LIBRARY_DIR"); v != "" {
		cfg.LibraryDir = v
	}
	if v := os.Getenv("MUXCORE_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if cfg.LibraryDir == "" {
		cfg.LibraryDir = cfg.DataDir
	}
	imageDir := os.Getenv("AUDIOBOOKS_IMAGE_DIR")
	if imageDir == "" {
		imageDir = filepath.Join(cfg.DataDir, "images")
	}
	return &Module{
		id: cfg.ID, grpcAddr: cfg.GRPCAddr, httpAddr: cfg.HTTPAddr,
		dataDir: cfg.DataDir, libraryDir: cfg.LibraryDir, imageDir: imageDir,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID: m.id, Name: "Audiobook Manager", Version: "0.2.0",
		Roles:        []string{"media", "audiobooks"},
		Description:  "Audiobook library manager with SQLite persistence",
		Capabilities: []string{"media.audiobooks", "audiobooks", "settings"},
		HTTPAddr:     m.httpAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	if err := os.MkdirAll(m.dataDir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(m.libraryDir, 0o700); err != nil {
		return fmt.Errorf("create library dir: %w", err)
	}
	dbPath := filepath.Join(m.dataDir, "audiobooks.db")
	store, err := OpenStore(ctx, dbPath)
	if err != nil {
		return err
	}
	m.store = store
	slog.Info("audiobook library store open", "db", dbPath, "library", m.libraryDir)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	if m.store == nil {
		return fmt.Errorf("store not initialized")
	}
	lc := &net.ListenConfig{}
	lis, err := lc.Listen(ctx, "tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC %s: %w", m.grpcAddr, err)
	}
	m.lis = lis
	m.grpcAddr = lis.Addr().String()
	m.grpcSrv = grpc.NewServer()
	abv1.RegisterAudiobookManagementServiceServer(m.grpcSrv, &abServer{m: m})
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)
	go func() {
		slog.Info("audiobooks gRPC listening", "addr", m.grpcAddr)
		if serveErr := m.grpcSrv.Serve(lis); serveErr != nil {
			slog.Error("gRPC serve", "error", serveErr)
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	m.registerAudiobooksHTTPAPI(mux)
	mux.HandleFunc("/images/", func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/images/", http.FileServer(http.Dir(m.getImageDir()))).ServeHTTP(w, r)
	})
	httpLis, err := lc.Listen(ctx, "tcp", m.httpAddr)
	if err != nil {
		return fmt.Errorf("listen HTTP %s: %w", m.httpAddr, err)
	}
	m.httpAddr = httpLis.Addr().String()
	m.httpSrv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		slog.Info("health listening", "addr", m.httpAddr)
		if err := m.httpSrv.Serve(httpLis); err != nil && err != http.ErrServerClosed {
			slog.Error("health serve", "error", err)
		}
	}()
	if _, err := m.ScanLibrary(ctx); err != nil {
		return fmt.Errorf("startup library scan: %w", err)
	}
	return nil
}

// GRPCListenAddr returns the bound gRPC address after Start.
func (m *Module) GRPCListenAddr() string { return m.grpcAddr }

// HTTPListenAddr returns the bound health/HTTP API address after Start.
func (m *Module) HTTPListenAddr() string { return m.httpAddr }

// LibraryDir returns the configured library root.
func (m *Module) LibraryDir() string { return m.libraryRoot() }

func (m *Module) libraryRoot() string {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return m.libraryDir
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	if m.httpSrv != nil {
		_ = m.httpSrv.Shutdown(ctx)
	}
	if m.store != nil {
		_ = m.store.Close()
		m.store = nil
	}
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	if m.store == nil {
		return fmt.Errorf("store not open")
	}
	return m.store.Ping(ctx)
}

// ScanLibrary scans the configured library root into SQLite.
func (m *Module) ScanLibrary(ctx context.Context) (*ScanResult, error) {
	m.cfgMu.RLock()
	root := m.libraryDir
	store := m.store
	m.cfgMu.RUnlock()
	if store == nil {
		return nil, fmt.Errorf("store not open")
	}
	return store.ScanLibraryRoot(ctx, root)
}

func (m *Module) deleteFilesUnderLibrary(files []*AudiobookFile) error {
	root := m.libraryRoot()
	for _, f := range files {
		abs, err := pathUnderRoot(root, f.Path)
		if err != nil {
			continue
		}
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete file %q: %w", abs, err)
		}
	}
	return nil
}

type abServer struct {
	abv1.UnimplementedAudiobookManagementServiceServer
	m *Module
}

func (s *abServer) AddAuthor(ctx context.Context, req *abv1.AddAuthorRequest) (*abv1.AddAuthorResponse, error) {
	a, err := s.m.store.AddAuthor(ctx, Author{Name: req.GetName(), Monitored: req.GetMonitored(), Path: req.GetPath()})
	if err != nil {
		return nil, err
	}
	return &abv1.AddAuthorResponse{Author: toPBAuthor(a)}, nil
}

func (s *abServer) GetAuthor(ctx context.Context, req *abv1.GetAuthorRequest) (*abv1.GetAuthorResponse, error) {
	a, err := s.m.store.GetAuthor(ctx, req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &abv1.GetAuthorResponse{Author: toPBAuthor(a)}, nil
}

func (s *abServer) ListAuthors(ctx context.Context, req *abv1.ListAuthorsRequest) (*abv1.ListAuthorsResponse, error) {
	items, err := s.m.store.ListAuthors(ctx, req.GetQuery())
	if err != nil {
		return nil, err
	}
	out := make([]*abv1.Author, 0, len(items))
	for _, a := range items {
		out = append(out, toPBAuthor(a))
	}
	return &abv1.ListAuthorsResponse{Authors: out}, nil
}

func (s *abServer) UpdateAuthor(ctx context.Context, req *abv1.UpdateAuthorRequest) (*abv1.UpdateAuthorResponse, error) {
	var name, path *string
	var monitored *bool
	if req.Name != nil {
		name = req.Name
	}
	if req.Path != nil {
		path = req.Path
	}
	if req.Monitored != nil {
		monitored = req.Monitored
	}
	a, err := s.m.store.UpdateAuthor(ctx, req.GetId(), name, path, monitored)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &abv1.UpdateAuthorResponse{Author: toPBAuthor(a)}, nil
}

func (s *abServer) RemoveAuthor(ctx context.Context, req *abv1.RemoveAuthorRequest) (*abv1.RemoveAuthorResponse, error) {
	if req.GetDeleteFiles() {
		files, err := s.m.store.ListAudiobookFilesByAuthor(ctx, req.GetId())
		if err != nil {
			return nil, err
		}
		if err := s.m.deleteFilesUnderLibrary(files); err != nil {
			return nil, err
		}
	}
	if err := s.m.store.RemoveAuthor(ctx, req.GetId()); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &abv1.RemoveAuthorResponse{Success: true}, nil
}

func (s *abServer) AddAudiobook(ctx context.Context, req *abv1.AddAudiobookRequest) (*abv1.AddAudiobookResponse, error) {
	ab, err := s.m.store.AddAudiobook(ctx, Audiobook{
		AuthorID: req.GetAuthorId(), Title: req.GetTitle(), Narrator: req.GetNarrator(),
		ASIN: req.GetAsin(), Year: req.GetYear(), DurationSeconds: req.GetDurationSeconds(),
		Monitored: req.GetMonitored(),
	})
	if err != nil {
		return nil, err
	}
	return &abv1.AddAudiobookResponse{Audiobook: toPBAudiobook(ab, nil)}, nil
}

func (s *abServer) GetAudiobook(ctx context.Context, req *abv1.GetAudiobookRequest) (*abv1.GetAudiobookResponse, error) {
	ab, err := s.m.store.GetAudiobook(ctx, req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	files, err := s.m.store.ListAudiobookFiles(ctx, ab.ID)
	if err != nil {
		return nil, err
	}
	return &abv1.GetAudiobookResponse{Audiobook: toPBAudiobook(ab, files)}, nil
}

func (s *abServer) ListAudiobooks(ctx context.Context, req *abv1.ListAudiobooksRequest) (*abv1.ListAudiobooksResponse, error) {
	items, err := s.m.store.ListAudiobooks(ctx, req.GetAuthorId())
	if err != nil {
		return nil, err
	}
	out := make([]*abv1.Audiobook, 0, len(items))
	for _, ab := range items {
		files, _ := s.m.store.ListAudiobookFiles(ctx, ab.ID)
		out = append(out, toPBAudiobook(ab, files))
	}
	return &abv1.ListAudiobooksResponse{Audiobooks: out}, nil
}

func (s *abServer) UpdateAudiobook(ctx context.Context, req *abv1.UpdateAudiobookRequest) (*abv1.UpdateAudiobookResponse, error) {
	var title, narrator, asin *string
	var year *int32
	var monitored *bool
	if req.Title != nil {
		title = req.Title
	}
	if req.Narrator != nil {
		narrator = req.Narrator
	}
	if req.Asin != nil {
		asin = req.Asin
	}
	if req.Year != nil {
		year = req.Year
	}
	if req.Monitored != nil {
		monitored = req.Monitored
	}
	ab, err := s.m.store.UpdateAudiobook(ctx, req.GetId(), title, narrator, asin, year, monitored)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	files, _ := s.m.store.ListAudiobookFiles(ctx, ab.ID)
	return &abv1.UpdateAudiobookResponse{Audiobook: toPBAudiobook(ab, files)}, nil
}

func (m *Module) removeAudiobook(ctx context.Context, id string, deleteFiles bool) error {
	if deleteFiles {
		files, err := m.store.ListAudiobookFiles(ctx, id)
		if err != nil {
			return err
		}
		if err := m.deleteFilesUnderLibrary(files); err != nil {
			return err
		}
	}
	return m.store.RemoveAudiobook(ctx, id)
}

func (s *abServer) RemoveAudiobook(ctx context.Context, req *abv1.RemoveAudiobookRequest) (*abv1.RemoveAudiobookResponse, error) {
	if err := s.m.removeAudiobook(ctx, req.GetId(), req.GetDeleteFiles()); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &abv1.RemoveAudiobookResponse{Success: true}, nil
}

func (s *abServer) ScanLibrary(ctx context.Context, _ *abv1.ScanLibraryRequest) (*abv1.ScanLibraryResponse, error) {
	res, err := s.m.ScanLibrary(ctx)
	if err != nil {
		return nil, err
	}
	return &abv1.ScanLibraryResponse{
		FilesFound: clampInt32(res.FilesFound), FilesImported: clampInt32(res.FilesImported),
		FilesSkipped: clampInt32(res.FilesSkipped), FilesRemoved: clampInt32(res.FilesRemoved),
	}, nil
}

func (s *abServer) ListAudiobookFiles(ctx context.Context, req *abv1.ListAudiobookFilesRequest) (*abv1.ListAudiobookFilesResponse, error) {
	files, err := s.m.store.ListAudiobookFiles(ctx, req.GetAudiobookId())
	if err != nil {
		return nil, err
	}
	out := make([]*abv1.AudiobookFile, 0, len(files))
	for _, f := range files {
		out = append(out, toPBFile(f))
	}
	return &abv1.ListAudiobookFilesResponse{Files: out}, nil
}

func (s *abServer) ListMissing(ctx context.Context, req *abv1.ListMissingRequest) (*abv1.ListMissingResponse, error) {
	page := int(req.GetPage())
	pageSize := int(req.GetPageSize())
	items, total, err := s.m.store.ListMissingAudiobooks(ctx, page, pageSize)
	if err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	out := make([]*abv1.MissingAudiobookItem, 0, len(items))
	for _, it := range items {
		out = append(out, &abv1.MissingAudiobookItem{
			AudiobookId: it.AudiobookID, AuthorId: it.AuthorID,
			Title: it.Title, AuthorName: it.AuthorName, Year: it.Year,
		})
	}
	return &abv1.ListMissingResponse{
		Items: out, Total: clampInt32(total), Page: clampInt32(page), PageSize: clampInt32(pageSize),
	}, nil
}

func (s *abServer) ImportAudiobookFile(ctx context.Context, req *abv1.ImportAudiobookFileRequest) (*abv1.ImportAudiobookFileResponse, error) {
	root := s.m.libraryRoot()
	abs, err := pathUnderRoot(root, req.GetPath())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	f, err := s.m.store.ImportAudiobookFile(ctx, req.GetAudiobookId(), abs, "")
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &abv1.ImportAudiobookFileResponse{File: toPBFile(f)}, nil
}

func toPBAuthor(a *Author) *abv1.Author {
	return &abv1.Author{Id: a.ID, Name: a.Name, Monitored: a.Monitored, Path: a.Path}
}

func toPBFile(f *AudiobookFile) *abv1.AudiobookFile {
	return &abv1.AudiobookFile{
		Id: f.ID, AudiobookId: f.AudiobookID, Title: f.Title, Path: f.Path,
	}
}

func toPBAudiobook(a *Audiobook, files []*AudiobookFile) *abv1.Audiobook {
	out := &abv1.Audiobook{
		Id: a.ID, AuthorId: a.AuthorID, Title: a.Title, Narrator: a.Narrator,
		Asin: a.ASIN, Year: a.Year, DurationSeconds: a.DurationSeconds, Monitored: a.Monitored,
	}
	if len(files) > 0 {
		out.Files = make([]*abv1.AudiobookFile, 0, len(files))
		for _, f := range files {
			out.Files = append(out.Files, toPBFile(f))
		}
	}
	return out
}
