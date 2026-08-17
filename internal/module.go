package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	abv1 "github.com/Muxcore-Media/media-audiobooks/proto/gen/muxcore/audiobooks/v1"
)

type Module struct {
	id         string
	grpcAddr   string
	httpAddr   string
	dataDir    string
	libraryDir string

	cfgMu sync.RWMutex
	store *Store

	grpcSrv *grpc.Server
	lis     net.Listener
	httpSrv *http.Server
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
		cfg.LibraryDir = filepath.Join(cfg.DataDir, "audiobooks")
	}
	return &Module{
		id: cfg.ID, grpcAddr: cfg.GRPCAddr, httpAddr: cfg.HTTPAddr,
		dataDir: cfg.DataDir, libraryDir: cfg.LibraryDir,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID: m.id, Name: "Audiobook Manager", Version: "0.2.0",
		Roles:        []string{"media", "audiobooks"},
		Description:  "Audiobook library manager with SQLite persistence",
		Capabilities: []string{"media.audiobooks", "audiobooks", "settings"},
		HTTPAddr:     m.grpcAddr,
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
	store, err := OpenStore(dbPath)
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
	lis, err := net.Listen("tcp", m.grpcAddr)
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
		if err := m.grpcSrv.Serve(lis); err != nil {
			slog.Error("gRPC serve", "error", err)
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	m.registerAudiobooksHTTPAPI(mux)
	httpLis, err := net.Listen("tcp", m.httpAddr)
	if err != nil {
		return fmt.Errorf("listen HTTP %s: %w", m.httpAddr, err)
	}
	m.httpAddr = httpLis.Addr().String()
	m.httpSrv = &http.Server{Handler: mux}
	go func() {
		slog.Info("health listening", "addr", m.httpAddr)
		if err := m.httpSrv.Serve(httpLis); err != nil && err != http.ErrServerClosed {
			slog.Error("health serve", "error", err)
		}
	}()
	return nil
}

// GRPCListenAddr returns the bound gRPC address after Start.
func (m *Module) GRPCListenAddr() string { return m.grpcAddr }

// HTTPListenAddr returns the bound health/HTTP API address after Start.
func (m *Module) HTTPListenAddr() string { return m.httpAddr }

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
	return nil
}

// ScanLibrary scans the configured library root into SQLite.
func (m *Module) ScanLibrary() (*ScanResult, error) {
	m.cfgMu.RLock()
	root := m.libraryDir
	store := m.store
	m.cfgMu.RUnlock()
	if store == nil {
		return nil, fmt.Errorf("store not open")
	}
	return store.ScanLibraryRoot(root)
}

type abServer struct {
	abv1.UnimplementedAudiobookManagementServiceServer
	m *Module
}

func (s *abServer) AddAuthor(_ context.Context, req *abv1.AddAuthorRequest) (*abv1.AddAuthorResponse, error) {
	a, err := s.m.store.AddAuthor(Author{Name: req.GetName(), Monitored: req.GetMonitored(), Path: req.GetPath()})
	if err != nil {
		return nil, err
	}
	return &abv1.AddAuthorResponse{Author: toPBAuthor(a)}, nil
}

func (s *abServer) GetAuthor(_ context.Context, req *abv1.GetAuthorRequest) (*abv1.GetAuthorResponse, error) {
	a, err := s.m.store.GetAuthor(req.GetId())
	if err != nil {
		return nil, err
	}
	return &abv1.GetAuthorResponse{Author: toPBAuthor(a)}, nil
}

func (s *abServer) ListAuthors(_ context.Context, req *abv1.ListAuthorsRequest) (*abv1.ListAuthorsResponse, error) {
	items, err := s.m.store.ListAuthors(req.GetQuery())
	if err != nil {
		return nil, err
	}
	out := make([]*abv1.Author, 0, len(items))
	for _, a := range items {
		out = append(out, toPBAuthor(a))
	}
	return &abv1.ListAuthorsResponse{Authors: out}, nil
}

func (s *abServer) RemoveAuthor(_ context.Context, req *abv1.RemoveAuthorRequest) (*abv1.RemoveAuthorResponse, error) {
	if err := s.m.store.RemoveAuthor(req.GetId()); err != nil {
		return nil, err
	}
	return &abv1.RemoveAuthorResponse{Success: true}, nil
}

func (s *abServer) AddAudiobook(_ context.Context, req *abv1.AddAudiobookRequest) (*abv1.AddAudiobookResponse, error) {
	ab, err := s.m.store.AddAudiobook(Audiobook{
		AuthorID: req.GetAuthorId(), Title: req.GetTitle(), Narrator: req.GetNarrator(),
		ASIN: req.GetAsin(), Year: req.GetYear(), DurationSeconds: req.GetDurationSeconds(),
		Monitored: req.GetMonitored(),
	})
	if err != nil {
		return nil, err
	}
	return &abv1.AddAudiobookResponse{Audiobook: toPBAudiobook(ab)}, nil
}

func (s *abServer) ListAudiobooks(_ context.Context, req *abv1.ListAudiobooksRequest) (*abv1.ListAudiobooksResponse, error) {
	items, err := s.m.store.ListAudiobooks(req.GetAuthorId())
	if err != nil {
		return nil, err
	}
	out := make([]*abv1.Audiobook, 0, len(items))
	for _, ab := range items {
		out = append(out, toPBAudiobook(ab))
	}
	return &abv1.ListAudiobooksResponse{Audiobooks: out}, nil
}

func toPBAuthor(a *Author) *abv1.Author {
	return &abv1.Author{Id: a.ID, Name: a.Name, Monitored: a.Monitored, Path: a.Path}
}

func toPBAudiobook(a *Audiobook) *abv1.Audiobook {
	return &abv1.Audiobook{
		Id: a.ID, AuthorId: a.AuthorID, Title: a.Title, Narrator: a.Narrator,
		Asin: a.ASIN, Year: a.Year, DurationSeconds: a.DurationSeconds, Monitored: a.Monitored,
	}
}
