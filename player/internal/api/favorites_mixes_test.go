package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFavoritesTrackArtistAlbumLifecycle(t *testing.T) {
	server := openTestServer(t)

	add := httptest.NewRequest("POST", "/api/favorites", bytes.NewReader([]byte(`{"type":"track","track_id":11}`)))
	rec := httptest.NewRecorder()
	server.handleFavoritesAdd(rec, add)
	if rec.Code != 200 {
		t.Fatalf("add track status=%d body=%s", rec.Code, rec.Body.String())
	}

	status := httptest.NewRequest("GET", "/api/favorites/status?type=track&track_id=11", nil)
	rec = httptest.NewRecorder()
	server.handleFavoritesStatus(rec, status)
	var st map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st["favorited"] != true {
		t.Fatalf("status=%v", st)
	}

	toggle := httptest.NewRequest("POST", "/api/favorites/toggle", bytes.NewReader([]byte(`{"type":"artist","artist":"Artist"}`)))
	rec = httptest.NewRecorder()
	server.handleFavoritesToggle(rec, toggle)
	if rec.Code != 200 {
		t.Fatalf("toggle artist status=%d body=%s", rec.Code, rec.Body.String())
	}

	toggle = httptest.NewRequest("POST", "/api/favorites/toggle", bytes.NewReader([]byte(`{"type":"album","artist":"Artist","album":"Album"}`)))
	rec = httptest.NewRecorder()
	server.handleFavoritesToggle(rec, toggle)
	if rec.Code != 200 {
		t.Fatalf("toggle album status=%d body=%s", rec.Code, rec.Body.String())
	}

	list := httptest.NewRequest("GET", "/api/favorites", nil)
	rec = httptest.NewRecorder()
	server.handleFavoritesList(rec, list)
	var body struct {
		Count  int `json:"count"`
		Counts struct {
			Tracks  int `json:"tracks"`
			Artists int `json:"artists"`
			Albums  int `json:"albums"`
		} `json:"counts"`
		Tracks  []any `json:"tracks"`
		Artists []any `json:"artists"`
		Albums  []any `json:"albums"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 1 || body.Counts.Artists != 1 || body.Counts.Albums != 1 {
		t.Fatalf("list counts unexpected: %+v", body)
	}

	remove := httptest.NewRequest("POST", "/api/favorites/remove", bytes.NewReader([]byte(`{"type":"track","track_id":11}`)))
	rec = httptest.NewRecorder()
	server.handleFavoritesRemove(rec, remove)
	if rec.Code != 200 {
		t.Fatalf("remove status=%d", rec.Code)
	}
	if server.Store.FavoritesHas(11) {
		t.Fatal("track should be removed")
	}

	// toggle off artist
	toggle = httptest.NewRequest("POST", "/api/favorites/toggle", bytes.NewReader([]byte(`{"type":"artist","artist":"Artist"}`)))
	rec = httptest.NewRecorder()
	server.handleFavoritesToggle(rec, toggle)
	if server.Store.FavArtistHas("Artist") {
		t.Fatal("artist should be toggled off")
	}
}

func TestFavoritesValidationErrors(t *testing.T) {
	server := openTestServer(t)
	req := httptest.NewRequest("GET", "/api/favorites/status?type=track", nil)
	rec := httptest.NewRecorder()
	server.handleFavoritesStatus(rec, req)
	if rec.Code != 400 {
		t.Fatalf("missing track_id status=%d", rec.Code)
	}

	req = httptest.NewRequest("POST", "/api/favorites/toggle", bytes.NewReader([]byte(`{"type":"nope"}`)))
	rec = httptest.NewRecorder()
	server.handleFavoritesToggle(rec, req)
	if rec.Code != 400 {
		t.Fatalf("bad type status=%d", rec.Code)
	}
}

func TestLaterAddListRemoveAndMixPlay(t *testing.T) {
	server := openTestServer(t)

	add := httptest.NewRequest("POST", "/api/later", bytes.NewReader([]byte(`{"track_id":22}`)))
	rec := httptest.NewRecorder()
	server.handleLaterAdd(rec, add)
	if rec.Code != 200 {
		t.Fatalf("later add status=%d body=%s", rec.Code, rec.Body.String())
	}

	list := httptest.NewRequest("GET", "/api/later", nil)
	rec = httptest.NewRecorder()
	server.handleLaterList(rec, list)
	var body struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 1 {
		t.Fatalf("later count=%d", body.Count)
	}

	play := httptest.NewRequest("POST", "/api/mixes/later/play", nil)
	play.SetPathValue("kind", "later")
	rec = httptest.NewRecorder()
	server.handleMixPlay(rec, play)
	if rec.Code != 200 {
		t.Fatalf("later play status=%d body=%s", rec.Code, rec.Body.String())
	}

	remove := httptest.NewRequest("POST", "/api/later/remove", bytes.NewReader([]byte(`{"track_id":22}`)))
	rec = httptest.NewRecorder()
	server.handleLaterRemove(rec, remove)
	if server.Store.LaterCount() != 0 {
		t.Fatal("later should be empty")
	}
}

func TestMixesShelfAndFavoritesPlay(t *testing.T) {
	server := openTestServer(t)
	if err := server.Store.FavoritesAdd(11); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Store.DB.Exec(`
INSERT INTO playlists(id, kind, name, created_at) VALUES (1,'daily','Daily','` + time.Now().UTC().Format(time.RFC3339Nano) + `');
INSERT INTO playlist_tracks(playlist_id, position, track_id, explanation) VALUES (1,0,11,''),(1,1,22,'')`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/mixes", nil)
	rec := httptest.NewRecorder()
	server.handleMixes(rec, req)
	if rec.Code != 200 {
		t.Fatalf("mixes status=%d", rec.Code)
	}
	var shelf struct {
		Mixes []map[string]any `json:"mixes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &shelf); err != nil {
		t.Fatal(err)
	}
	if len(shelf.Mixes) < 5 {
		t.Fatalf("expected mix shelf, got %d", len(shelf.Mixes))
	}

	play := httptest.NewRequest("POST", "/api/mixes/favorites/play", nil)
	play.SetPathValue("kind", "favorites")
	rec = httptest.NewRecorder()
	server.handleMixPlay(rec, play)
	if rec.Code != 200 {
		t.Fatalf("favorites play status=%d body=%s", rec.Code, rec.Body.String())
	}

	play = httptest.NewRequest("POST", "/api/mixes/daily/play", bytes.NewReader([]byte(`{"start_track_id":22}`)))
	play.SetPathValue("kind", "daily")
	rec = httptest.NewRecorder()
	server.handleMixPlay(rec, play)
	if rec.Code != 200 {
		t.Fatalf("daily mix play status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Index int   `json:"index"`
		Count int   `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Index != 1 || resp.Count != 2 {
		t.Fatalf("daily play index/count=%d/%d", resp.Index, resp.Count)
	}
}
