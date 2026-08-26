package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/torwin-job/musik/player/internal/db"
	"github.com/torwin-job/musik/player/internal/index"
)

func loadSimilarFixture(t *testing.T, server *Server) {
	t.Helper()
	idx := index.New(server.Cfg)
	rows := []db.TrackRow{
		{ID: 1, Artist: "Massive Attack", Album: "Mezzanine", Title: "Angel", Embedding: index.Float32Bytes([]float32{1, 0}), Dim: 2, FileMD5: "md5-a"},
		{ID: 2, Artist: "Massive Attack", Album: "Mezzanine", Title: "Teardrop", Embedding: index.Float32Bytes([]float32{0.95, 0.05}), Dim: 2},
		{ID: 3, Artist: "Portishead", Album: "Dummy", Title: "Glory Box", Embedding: index.Float32Bytes([]float32{0.9, 0.1}), Dim: 2},
		{ID: 4, Artist: "Portishead", Album: "Third", Title: "Machine Gun", Embedding: index.Float32Bytes([]float32{0.2, 0.8}), Dim: 2},
		{ID: 5, Artist: "Bjork", Album: "Homogenic", Title: "Joga", Embedding: index.Float32Bytes([]float32{0, 1}), Dim: 2},
		{ID: 6, Artist: "Massive Attack", Album: "Mezzanine", Title: "Angel", Embedding: index.Float32Bytes([]float32{0.99, 0.01}), Dim: 2, FileMD5: "md5-a"},
	}
	if err := idx.Load(rows); err != nil {
		t.Fatal(err)
	}
	server.Idx = idx
}

func TestHandleSimilarExcludesClones(t *testing.T) {
	server := openTestServer(t)
	loadSimilarFixture(t, server)

	req := httptest.NewRequest("GET", "/api/similar/1", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	server.handleSimilar(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for _, item := range out {
		if item.ID == 1 || item.ID == 6 {
			t.Fatalf("clone id %d should be excluded: %+v", item.ID, out)
		}
	}
	if len(out) == 0 {
		t.Fatal("expected similar tracks")
	}
	if out[0].ID != 2 && out[0].ID != 3 {
		t.Fatalf("top similar = %d, want near Mezzanine/Portishead", out[0].ID)
	}
}

func TestSimilarArtistsAndAlbums(t *testing.T) {
	server := openTestServer(t)
	loadSimilarFixture(t, server)

	req := httptest.NewRequest("GET", "/api/similar/artists?artist=Massive%20Attack", nil)
	rec := httptest.NewRecorder()
	server.handleSimilarArtists(rec, req)
	if rec.Code != 200 {
		t.Fatalf("artists status=%d body=%s", rec.Code, rec.Body.String())
	}
	var artists struct {
		Artists []struct {
			Artist string `json:"artist"`
		} `json:"artists"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &artists); err != nil {
		t.Fatal(err)
	}
	if len(artists.Artists) == 0 {
		t.Fatal("expected similar artists")
	}
	for _, a := range artists.Artists {
		if a.Artist == "Massive Attack" {
			t.Fatal("seed artist must be excluded")
		}
	}
	if artists.Artists[0].Artist != "Portishead" {
		t.Fatalf("top artist=%q, want Portishead", artists.Artists[0].Artist)
	}

	req = httptest.NewRequest("GET", "/api/similar/albums?artist=Massive%20Attack&album=Mezzanine", nil)
	rec = httptest.NewRecorder()
	server.handleSimilarAlbums(rec, req)
	if rec.Code != 200 {
		t.Fatalf("albums status=%d body=%s", rec.Code, rec.Body.String())
	}
	var albums struct {
		Albums []struct {
			Artist string `json:"artist"`
			Album  string `json:"album"`
		} `json:"albums"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &albums); err != nil {
		t.Fatal(err)
	}
	if len(albums.Albums) == 0 {
		t.Fatal("expected similar albums")
	}
	for _, a := range albums.Albums {
		if a.Album == "Mezzanine" && a.Artist == "Massive Attack" {
			t.Fatal("seed album must be excluded")
		}
	}
}

func TestRecommendSeedByTrackAndArtist(t *testing.T) {
	server := openTestServer(t)
	loadSimilarFixture(t, server)

	req := httptest.NewRequest("GET", "/api/recommend?type=track&track_id=1&limit=3", nil)
	rec := httptest.NewRecorder()
	server.handleRecommendSeed(rec, req)
	if rec.Code != 200 {
		t.Fatalf("track seed status=%d body=%s", rec.Code, rec.Body.String())
	}
	var trackResp struct {
		OK     bool `json:"ok"`
		Tracks []struct {
			ID int64 `json:"id"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &trackResp); err != nil {
		t.Fatal(err)
	}
	if !trackResp.OK || len(trackResp.Tracks) == 0 {
		t.Fatalf("unexpected track recommend: %+v", trackResp)
	}
	for _, tr := range trackResp.Tracks {
		if tr.ID == 1 {
			t.Fatal("seed track must be excluded")
		}
	}

	req = httptest.NewRequest("GET", "/api/recommend?type=artist&artist=Bjork", nil)
	rec = httptest.NewRecorder()
	server.handleRecommendSeed(rec, req)
	if rec.Code != 200 {
		t.Fatalf("artist seed status=%d body=%s", rec.Code, rec.Body.String())
	}
	var artistResp struct {
		Tracks []struct {
			ID     int64  `json:"id"`
			Artist string `json:"artist"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &artistResp); err != nil {
		t.Fatal(err)
	}
	for _, tr := range artistResp.Tracks {
		if tr.ID == 5 || tr.Artist == "Bjork" {
			t.Fatalf("seed artist tracks leaked: %+v", tr)
		}
	}
}

func TestRecommendationMetricsIncludesLatency(t *testing.T) {
	server := openTestServer(t)
	loadSimilarFixture(t, server)

	req := httptest.NewRequest("GET", "/api/similar/1", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	server.handleSimilar(rec, req)
	if rec.Code != 200 {
		t.Fatalf("similar status=%d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/api/metrics/recommendations", nil)
	rec = httptest.NewRecorder()
	server.handleRecommendationMetrics(rec, req)
	if rec.Code != 200 {
		t.Fatalf("metrics status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Window    string                     `json:"window"`
		LatencyMS map[string]latencySummary `json:"latency_ms"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Window != "7d" {
		t.Fatalf("window=%q, want 7d", body.Window)
	}
	if body.LatencyMS["similar"].Count < 1 {
		t.Fatalf("expected similar latency samples, got %#v", body.LatencyMS)
	}
}

func TestRadioStartRecordsImpressions(t *testing.T) {
	server := openTestServer(t)
	loadSimilarFixture(t, server)

	req := httptest.NewRequest("POST", "/api/radio/start", nil)
	rec := httptest.NewRecorder()
	server.handleRadioStart(rec, req)
	if rec.Code != 200 {
		t.Fatalf("radio status=%d body=%s", rec.Code, rec.Body.String())
	}
	server.flushBackgroundIO()

	var n int
	if err := server.Store.DB.QueryRow(`SELECT COUNT(*) FROM recommendation_impressions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("radio queue refresh should persist recommendation impressions")
	}
}
