package api

import (
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/torwin-job/musik/player/internal/db"
	"github.com/torwin-job/musik/player/internal/index"
)

func TestImageContentType(t *testing.T) {
	tests := map[string]string{
		"a.jpg": "image/jpeg", "b.JPEG": "image/jpeg", "c.png": "image/png",
		"d.webp": "image/webp", "e.gif": "image/gif", "f.bin": "",
	}
	for path, want := range tests {
		if got := imageContentType(path); got != want {
			t.Fatalf("imageContentType(%q)=%q, want %q", path, got, want)
		}
	}
}

func TestResizeMaxShrinksAndKeepsSmall(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			src.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	out := resizeMax(src, 50)
	if out.Bounds().Dx() != 50 || out.Bounds().Dy() != 25 {
		t.Fatalf("resized=%v, want 50x25", out.Bounds())
	}
	small := image.NewRGBA(image.Rect(0, 0, 20, 20))
	if got := resizeMax(small, 50); got.Bounds() != small.Bounds() {
		t.Fatal("small image should stay unchanged")
	}
}

func TestHandleArtworkServesOriginalAndThumb(t *testing.T) {
	server := openTestServer(t)
	artPath := filepath.Join(t.TempDir(), "cover.png")
	img := image.NewRGBA(image.Rect(0, 0, 120, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 120; x++ {
			img.Set(x, y, color.RGBA{B: 200, A: 255})
		}
	}
	f, err := os.Create(artPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	idx := index.New(server.Cfg)
	rows := []db.TrackRow{
		{ID: 11, Title: "Track", Artist: "Artist", Album: "Album", Duration: 180,
			ArtworkPath: artPath, Embedding: index.Float32Bytes([]float32{1, 0}), Dim: 2},
		{ID: 22, Title: "Track", Artist: "Artist", Album: "Album", Duration: 180,
			Embedding: index.Float32Bytes([]float32{0, 1}), Dim: 2},
	}
	if err := idx.Load(rows); err != nil {
		t.Fatal(err)
	}
	server.Idx = idx

	req := httptest.NewRequest("GET", "/api/artwork/11", nil)
	req.SetPathValue("id", "11")
	rec := httptest.NewRecorder()
	server.handleArtwork(rec, req)
	if rec.Code != 200 {
		t.Fatalf("original status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type=%q, want image/png", ct)
	}
	if len(rec.Body.Bytes()) < 50 {
		t.Fatal("expected image bytes")
	}

	req = httptest.NewRequest("GET", "/api/artwork/11?w=64", nil)
	req.SetPathValue("id", "11")
	rec = httptest.NewRecorder()
	server.handleArtwork(rec, req)
	if rec.Code != 200 {
		t.Fatalf("thumb status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("thumb content-type=%q, want image/jpeg", ct)
	}
	thumb := filepath.Join(server.artThumbDir(), "11_w64.jpg")
	if _, err := os.Stat(thumb); err != nil {
		t.Fatalf("thumb file missing: %v", err)
	}

	// second call should reuse cached thumb
	req = httptest.NewRequest("GET", "/api/artwork/11?w=64", nil)
	req.SetPathValue("id", "11")
	rec = httptest.NewRecorder()
	server.handleArtwork(rec, req)
	if rec.Code != 200 {
		t.Fatalf("cached thumb status=%d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/api/artwork/22", nil)
	req.SetPathValue("id", "22")
	rec = httptest.NewRecorder()
	server.handleArtwork(rec, req)
	if rec.Code != 404 {
		t.Fatalf("missing artwork status=%d, want 404", rec.Code)
	}
}

func TestHandleArtworkBadID(t *testing.T) {
	server := openTestServer(t)
	req := httptest.NewRequest("GET", "/api/artwork/x", nil)
	req.SetPathValue("id", "x")
	rec := httptest.NewRecorder()
	server.handleArtwork(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestArtThumbDirFromDBPath(t *testing.T) {
	server := openTestServer(t)
	got := server.artThumbDir()
	want := filepath.Join(filepath.Dir(filepath.Dir(server.Cfg.DBPath)), "cache", "art")
	if got != want {
		t.Fatalf("artThumbDir=%q, want %q", got, want)
	}
}
