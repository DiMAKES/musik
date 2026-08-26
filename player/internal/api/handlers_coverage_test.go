package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCatalogArtistsAlbumsTrack(t *testing.T) {
	server := openTestServer(t)

	req := httptest.NewRequest("GET", "/api/artists", nil)
	rec := httptest.NewRecorder()
	server.handleArtists(rec, req)
	var artists struct {
		Count   int `json:"count"`
		Artists []struct {
			Artist string `json:"artist"`
			Tracks int    `json:"tracks"`
		} `json:"artists"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &artists); err != nil {
		t.Fatal(err)
	}
	if artists.Count != 1 || artists.Artists[0].Tracks != 3 {
		t.Fatalf("artists=%+v", artists)
	}

	req = httptest.NewRequest("GET", "/api/albums", nil)
	rec = httptest.NewRecorder()
	server.handleAlbums(rec, req)
	var albums struct {
		Count  int `json:"count"`
		Albums []struct {
			Album string `json:"album"`
		} `json:"albums"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &albums); err != nil {
		t.Fatal(err)
	}
	if albums.Count != 1 || albums.Albums[0].Album != "Album" {
		t.Fatalf("albums=%+v", albums)
	}

	req = httptest.NewRequest("GET", "/api/track/11", nil)
	req.SetPathValue("id", "11")
	rec = httptest.NewRecorder()
	server.handleTrack(rec, req)
	if rec.Code != 200 {
		t.Fatalf("track status=%d", rec.Code)
	}
	var track map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &track); err != nil {
		t.Fatal(err)
	}
	if track["id"].(float64) != 11 {
		t.Fatalf("track=%v", track)
	}

	req = httptest.NewRequest("GET", "/api/track/999", nil)
	req.SetPathValue("id", "999")
	rec = httptest.NewRecorder()
	server.handleTrack(rec, req)
	if rec.Code != 404 {
		t.Fatalf("missing track status=%d", rec.Code)
	}
}

func TestPlayByArtistAndSessionJump(t *testing.T) {
	server := openTestServer(t)

	req := httptest.NewRequest("POST", "/api/play", bytes.NewReader([]byte(`{"artist":"Artist","start_track_id":33}`)))
	rec := httptest.NewRecorder()
	server.handlePlay(rec, req)
	if rec.Code != 200 {
		t.Fatalf("play status=%d body=%s", rec.Code, rec.Body.String())
	}
	var play struct {
		SessionID string `json:"session_id"`
		Index     int    `json:"index"`
		Count     int    `json:"count"`
		Fixed     bool   `json:"fixed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &play); err != nil {
		t.Fatal(err)
	}
	if !play.Fixed || play.Count != 3 || play.Index != 2 {
		t.Fatalf("play=%+v", play)
	}

	jump := httptest.NewRequest("POST", "/api/session/jump", bytes.NewReader([]byte(
		`{"session_id":"`+play.SessionID+`","track_id":11}`,
	)))
	rec = httptest.NewRecorder()
	server.handleSessionJump(rec, jump)
	if rec.Code != 200 {
		t.Fatalf("jump status=%d body=%s", rec.Code, rec.Body.String())
	}
	var jumped struct {
		Index int `json:"index"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &jumped); err != nil {
		t.Fatal(err)
	}
	if jumped.Index != 0 {
		t.Fatalf("jumped index=%d", jumped.Index)
	}
}

func TestDailyAndPlaylistLatest(t *testing.T) {
	server := openTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.Store.DB.Exec(`
INSERT INTO playlists(id, kind, name, created_at) VALUES (5,'daily','D',?), (6,'weekly','W',?);
INSERT INTO playlist_tracks(playlist_id, position, track_id, explanation)
VALUES (5,0,11,''),(5,1,22,''),(6,0,33,'')`, now, now); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/daily/today", nil)
	rec := httptest.NewRecorder()
	server.handleDailyToday(rec, req)
	var today struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &today); err != nil {
		t.Fatal(err)
	}
	if !today.OK {
		t.Fatal("daily today should be ok")
	}

	req = httptest.NewRequest("POST", "/api/daily/play", nil)
	rec = httptest.NewRecorder()
	server.handleDailyPlay(rec, req)
	if rec.Code != 200 {
		t.Fatalf("daily play status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest("GET", "/api/playlists/weekly/latest", nil)
	req.SetPathValue("kind", "weekly")
	rec = httptest.NewRecorder()
	server.handlePlaylistLatest(rec, req)
	if rec.Code != 200 {
		t.Fatalf("playlist latest status=%d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/api/playlists/missing/latest", nil)
	req.SetPathValue("kind", "missing")
	rec = httptest.NewRecorder()
	server.handlePlaylistLatest(rec, req)
	if rec.Code != 404 {
		t.Fatalf("missing playlist status=%d", rec.Code)
	}
}

func TestLyricsAbsentAndPresent(t *testing.T) {
	server := openTestServer(t)

	req := httptest.NewRequest("GET", "/api/lyrics/11", nil)
	req.SetPathValue("id", "11")
	rec := httptest.NewRecorder()
	server.handleTrackLyrics(rec, req)
	var absent map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &absent); err != nil {
		t.Fatal(err)
	}
	if absent["status"] != "absent" {
		t.Fatalf("absent=%v", absent)
	}

	if _, err := server.Store.DB.Exec(`
INSERT INTO lyrics(track_id, plain_lyrics, synced_lyrics, source, source_id, instrumental, status, updated_at)
VALUES (11,'hello','','local','',0,'ok',?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest("GET", "/api/lyrics/11", nil)
	req.SetPathValue("id", "11")
	rec = httptest.NewRecorder()
	server.handleTrackLyrics(rec, req)
	var ly struct {
		PlainLyrics string `json:"plain_lyrics"`
		Status      string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ly); err != nil {
		t.Fatal(err)
	}
	if ly.PlainLyrics != "hello" || ly.Status != "ok" {
		t.Fatalf("lyrics=%+v", ly)
	}
}

func TestJobsLocalFallbackListGet(t *testing.T) {
	server := openTestServer(t)

	req := httptest.NewRequest("POST", "/api/jobs/mix_pack", nil)
	req.SetPathValue("kind", "mix_pack")
	rec := httptest.NewRecorder()
	server.handleEnqueueJob(rec, req)
	if rec.Code != 200 {
		t.Fatalf("enqueue status=%d body=%s", rec.Code, rec.Body.String())
	}
	var enq struct {
		OK     bool  `json:"ok"`
		ID     int64 `json:"id"`
		Status string `json:"status"`
		Via    string `json:"via"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &enq); err != nil {
		t.Fatal(err)
	}
	if !enq.OK || enq.ID == 0 || enq.Via != "local_db" {
		t.Fatalf("enqueue=%+v", enq)
	}

	req = httptest.NewRequest("GET", "/api/jobs", nil)
	rec = httptest.NewRecorder()
	server.handleListJobs(rec, req)
	var listed struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Count < 1 {
		t.Fatal("expected listed jobs")
	}

	req = httptest.NewRequest("GET", "/api/jobs/1", nil)
	req.SetPathValue("id", "1")
	// use actual id
	req.SetPathValue("id", jsonInt(enq.ID))
	rec = httptest.NewRecorder()
	server.handleGetJob(rec, req)
	if rec.Code != 200 {
		t.Fatalf("get job status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest("POST", "/api/library/rescan", nil)
	rec = httptest.NewRecorder()
	server.handleLibraryRescan(rec, req)
	if rec.Code != 200 {
		t.Fatalf("rescan status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestDiscoverTipsProfileHealthStatusMetrics(t *testing.T) {
	server := openTestServer(t)
	if _, err := server.Store.DB.Exec(`
INSERT INTO discover_tips(kind, artist, album, score, track_ids_json, explanation, created_at)
VALUES ('new_album','Artist','Album',1.5,'[11,22]','fresh',?),
       ('resurfaced','Artist','Album',0.5,'[33]','old',?)`,
		time.Now().UTC().Format(time.RFC3339Nano),
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/discover/albums", nil)
	rec := httptest.NewRecorder()
	server.handleDiscoverAlbums(rec, req)
	var tips struct {
		Tips []map[string]any `json:"tips"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tips); err != nil {
		t.Fatal(err)
	}
	if len(tips.Tips) != 1 {
		t.Fatalf("new album tips=%d", len(tips.Tips))
	}

	req = httptest.NewRequest("GET", "/api/discover/resurfaced", nil)
	rec = httptest.NewRecorder()
	server.handleDiscoverResurfaced(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &tips); err != nil {
		t.Fatal(err)
	}
	if len(tips.Tips) != 1 {
		t.Fatalf("resurfaced tips=%d", len(tips.Tips))
	}

	req = httptest.NewRequest("GET", "/api/profile", nil)
	rec = httptest.NewRecorder()
	server.handleProfile(rec, req)
	if rec.Code != 200 {
		t.Fatalf("profile status=%d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/api/health", nil)
	rec = httptest.NewRecorder()
	server.handleHealth(rec, req)
	var health map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if health["ok"] != true {
		t.Fatalf("health=%v", health)
	}

	req = httptest.NewRequest("GET", "/api/status", nil)
	rec = httptest.NewRecorder()
	server.handleStatus(rec, req)
	var status map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status["tracks"].(float64) != 3 {
		t.Fatalf("status tracks=%v", status["tracks"])
	}

	req = httptest.NewRequest("GET", "/api/metrics/weekly", nil)
	rec = httptest.NewRecorder()
	server.handleWeeklyMetrics(rec, req)
	if rec.Code != 200 {
		t.Fatalf("weekly metrics status=%d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/api/openapi.json", nil)
	rec = httptest.NewRecorder()
	server.handleOpenAPI(rec, req)
	if rec.Code != 200 || len(rec.Body.Bytes()) < 10 {
		t.Fatalf("openapi status=%d len=%d", rec.Code, rec.Body.Len())
	}

	req = httptest.NewRequest("GET", "/manifest.webmanifest", nil)
	rec = httptest.NewRecorder()
	server.handleManifest(rec, req)
	if rec.Code != 200 || rec.Header().Get("Content-Type") == "" {
		t.Fatalf("manifest status=%d ct=%s", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestRecommendFavoritesEmpty(t *testing.T) {
	server := openTestServer(t)
	req := httptest.NewRequest("GET", "/api/recommend/favorites", nil)
	rec := httptest.NewRecorder()
	server.handleRecommendFavorites(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["empty"] != true {
		t.Fatalf("expected empty favorites recommend: %#v", body)
	}
}
