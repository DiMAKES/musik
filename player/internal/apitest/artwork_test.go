package apitest

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/torwin-job/musik/player/internal/db"
	"github.com/torwin-job/musik/player/internal/index"
)

func TestHandleArtworkServesOriginalAndThumb(t *testing.T) {
	server := openTestServer(t)
	artPath := filepath.Join(t.TempDir(), "cover.png")
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 800; x++ {
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

	loadIndex(t, server, []db.TrackRow{
		{ID: 11, Title: "Track", Artist: "Artist", Album: "Album", Duration: 180,
			ArtworkPath: artPath, Embedding: index.Float32Bytes([]float32{1, 0}), Dim: 2},
		{ID: 22, Title: "Track", Artist: "Artist", Album: "Album", Duration: 180,
			Embedding: index.Float32Bytes([]float32{0, 1}), Dim: 2},
	})

	rec := serve(server, httptest.NewRequest("GET", "/api/artwork/11", nil))
	if rec.Code != 200 {
		t.Fatalf("default status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("default content-type=%q, want image/jpeg", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, max-age=2592000" {
		t.Fatalf("default cache-control=%q", cc)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("default jpeg: %v", err)
	}
	if decoded.Bounds().Dx() != 640 {
		t.Fatalf("default width=%d, want 640", decoded.Bounds().Dx())
	}
	thumb640 := filepath.Join(filepath.Dir(filepath.Dir(server.Cfg.DBPath)), "cache", "art", "11_w640.jpg")
	if _, err := os.Stat(thumb640); err != nil {
		t.Fatalf("640 thumb file missing: %v", err)
	}

	rec = serve(server, httptest.NewRequest("GET", "/api/artwork/11?w=64", nil))
	if rec.Code != 200 {
		t.Fatalf("snapped thumb status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("thumb content-type=%q, want image/jpeg", ct)
	}
	thumb96 := filepath.Join(filepath.Dir(filepath.Dir(server.Cfg.DBPath)), "cache", "art", "11_w96.jpg")
	if _, err := os.Stat(thumb96); err != nil {
		t.Fatalf("96 thumb file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(thumb96), "11_w64.jpg")); err == nil {
		t.Fatal("expected w=64 to snap to 96, not write 11_w64.jpg")
	}

	rec = serve(server, httptest.NewRequest("GET", "/api/artwork/11?w=200", nil))
	if rec.Code != 200 {
		t.Fatalf("256 thumb status=%d", rec.Code)
	}
	thumb256 := filepath.Join(filepath.Dir(filepath.Dir(server.Cfg.DBPath)), "cache", "art", "11_w256.jpg")
	if _, err := os.Stat(thumb256); err != nil {
		t.Fatalf("256 thumb file missing: %v", err)
	}

	rec = serve(server, httptest.NewRequest("GET", "/api/artwork/11?w=96", nil))
	if rec.Code != 200 {
		t.Fatalf("cached thumb status=%d", rec.Code)
	}

	gzipReq := httptest.NewRequest("GET", "/api/artwork/11?w=256", nil)
	gzipReq.Header.Set("Accept-Encoding", "gzip")
	rec = serve(server, gzipReq)
	if rec.Code != 200 {
		t.Fatalf("gzip thumb status=%d", rec.Code)
	}
	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Fatal("JPEG artwork must not be gzipped")
	}

	rec = serve(server, httptest.NewRequest("GET", "/api/artwork/11?full=1", nil))
	if rec.Code != 200 {
		t.Fatalf("full status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("full content-type=%q, want image/png", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, max-age=86400" {
		t.Fatalf("full cache-control=%q", cc)
	}

	rec = serve(server, httptest.NewRequest("GET", "/api/artwork/22", nil))
	if rec.Code != 404 {
		t.Fatalf("missing artwork status=%d, want 404", rec.Code)
	}
}

func TestHandleArtworkBadID(t *testing.T) {
	server := openTestServer(t)
	rec := serve(server, httptest.NewRequest("GET", "/api/artwork/x", nil))
	if rec.Code != 400 {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}
