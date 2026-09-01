package internal_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Muxcore-Media/media-audiobooks/internal"
	abv1 "github.com/Muxcore-Media/media-audiobooks/proto/gen/muxcore/audiobooks/v1"
)

func startGRPCModule(t *testing.T) (*internal.Module, abv1.AudiobookManagementServiceClient) {
	t.Helper()
	data := t.TempDir()
	lib := filepath.Join(data, "library")
	if err := copyTree(filepath.Join("testdata", "library"), lib); err != nil {
		t.Fatal(err)
	}
	m := internal.NewModule(internal.Config{
		DataDir: data, LibraryDir: lib,
		GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0",
	})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(t.Context()) })
	conn, err := grpc.NewClient(m.GRPCListenAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return m, abv1.NewAudiobookManagementServiceClient(conn)
}

func TestGRPCAuthorAudiobookLifecycle(t *testing.T) {
	_, cli := startGRPCModule(t)
	ctx := context.Background()

	addAu, err := cli.AddAuthor(ctx, &abv1.AddAuthorRequest{Name: "gRPC Author", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	getAu, err := cli.GetAuthor(ctx, &abv1.GetAuthorRequest{Id: addAu.GetAuthor().GetId()})
	if err != nil || getAu.GetAuthor().GetName() != "gRPC Author" {
		t.Fatalf("get author: %+v err=%v", getAu, err)
	}
	mon := false
	_, err = cli.UpdateAuthor(ctx, &abv1.UpdateAuthorRequest{Id: addAu.GetAuthor().GetId(), Monitored: &mon})
	if err != nil {
		t.Fatal(err)
	}

	addAb, err := cli.AddAudiobook(ctx, &abv1.AddAudiobookRequest{
		AuthorId: addAu.GetAuthor().GetId(), Title: "gRPC Book", Monitored: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	getAb, err := cli.GetAudiobook(ctx, &abv1.GetAudiobookRequest{Id: addAb.GetAudiobook().GetId()})
	if err != nil || getAb.GetAudiobook().GetTitle() != "gRPC Book" {
		t.Fatalf("get audiobook: %+v err=%v", getAb, err)
	}
	title := "Updated Book"
	_, err = cli.UpdateAudiobook(ctx, &abv1.UpdateAudiobookRequest{Id: addAb.GetAudiobook().GetId(), Title: &title})
	if err != nil {
		t.Fatal(err)
	}

	listAu, err := cli.ListAuthors(ctx, &abv1.ListAuthorsRequest{})
	if err != nil || len(listAu.GetAuthors()) < 1 {
		t.Fatalf("list authors: %+v err=%v", listAu, err)
	}
	listAb, err := cli.ListAudiobooks(ctx, &abv1.ListAudiobooksRequest{AuthorId: addAu.GetAuthor().GetId()})
	if err != nil || len(listAb.GetAudiobooks()) != 1 {
		t.Fatalf("list audiobooks: %+v err=%v", listAb, err)
	}

	if _, err := cli.RemoveAudiobook(ctx, &abv1.RemoveAudiobookRequest{Id: addAb.GetAudiobook().GetId()}); err != nil {
		t.Fatal(err)
	}
	if _, err := cli.RemoveAuthor(ctx, &abv1.RemoveAuthorRequest{Id: addAu.GetAuthor().GetId()}); err != nil {
		t.Fatal(err)
	}
}

func TestGRPCScanListMissingImport(t *testing.T) {
	m, cli := startGRPCModule(t)
	ctx := context.Background()

	scan, err := cli.ScanLibrary(ctx, &abv1.ScanLibraryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if scan.GetFilesFound() < 2 {
		t.Fatalf("scan: %+v", scan)
	}

	missing, err := cli.ListMissing(ctx, &abv1.ListMissingRequest{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	_ = missing

	listAb, err := cli.ListAudiobooks(ctx, &abv1.ListAudiobooksRequest{})
	if err != nil || len(listAb.GetAudiobooks()) == 0 {
		t.Fatal("expected scanned audiobooks")
	}
	ab := listAb.GetAudiobooks()[0]
	if len(ab.GetFiles()) < 1 {
		t.Fatal("expected files on list audiobooks")
	}

	files, err := cli.ListAudiobookFiles(ctx, &abv1.ListAudiobookFilesRequest{AudiobookId: ab.GetId()})
	if err != nil || len(files.GetFiles()) < 1 {
		t.Fatalf("list files: %+v err=%v", files, err)
	}

	stub := filepath.Join(m.LibraryDir(), "Fixture Author", "Fixture Book", "03-import.m4b")
	if err := os.WriteFile(stub, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	imp, err := cli.ImportAudiobookFile(ctx, &abv1.ImportAudiobookFileRequest{
		AudiobookId: ab.GetId(), Path: stub,
	})
	if err != nil {
		t.Fatal(err)
	}
	if imp.GetFile().GetId() == "" {
		t.Fatal("expected imported file id")
	}
}

func TestGRPCRemoveAuthorDeleteFiles(t *testing.T) {
	data := t.TempDir()
	lib := filepath.Join(data, "library")
	bookDir := filepath.Join(lib, "Del Author", "Del Book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(bookDir, "track.mp3")
	if err := os.WriteFile(stub, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := internal.NewModule(internal.Config{
		DataDir: data, LibraryDir: lib,
		GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0",
	})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(t.Context()) })

	conn, err := grpc.NewClient(m.GRPCListenAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	cli := abv1.NewAudiobookManagementServiceClient(conn)
	ctx := context.Background()

	if _, err := m.ScanLibrary(); err != nil {
		t.Fatal(err)
	}
	listAu, err := cli.ListAuthors(ctx, &abv1.ListAuthorsRequest{Query: "Del Author"})
	if err != nil || len(listAu.GetAuthors()) != 1 {
		t.Fatalf("authors: %+v err=%v", listAu, err)
	}
	if _, err := cli.RemoveAuthor(ctx, &abv1.RemoveAuthorRequest{
		Id: listAu.GetAuthors()[0].GetId(), DeleteFiles: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stub); !os.IsNotExist(err) {
		t.Fatalf("expected file deleted, stat err=%v", err)
	}

	outside := filepath.Join(data, "outside.mp3")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	au, err := cli.AddAuthor(ctx, &abv1.AddAuthorRequest{Name: "Keep Author", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	ab, err := cli.AddAudiobook(ctx, &abv1.AddAudiobookRequest{
		AuthorId: au.GetAuthor().GetId(), Title: "Keep Book", Monitored: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cli.ImportAudiobookFile(ctx, &abv1.ImportAudiobookFileRequest{
		AudiobookId: ab.GetAudiobook().GetId(), Path: outside,
	}); err == nil {
		t.Fatal("expected import outside library to fail")
	}
}
