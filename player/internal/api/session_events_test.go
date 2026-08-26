package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestSessionStartQueueNowAndEvents(t *testing.T) {
	server := openTestServer(t)

	seed := int64(11)
	body, _ := json.Marshal(map[string]any{"seed_track_id": seed})
	req := httptest.NewRequest("POST", "/api/session/start", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleSessionStart(rec, req)
	if rec.Code != 200 {
		t.Fatalf("session start status=%d body=%s", rec.Code, rec.Body.String())
	}
	var started struct {
		SessionID string `json:"session_id"`
		Current   struct {
			ID int64 `json:"id"`
		} `json:"current"`
		Queue []any `json:"queue"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.SessionID == "" || started.Current.ID != 11 {
		t.Fatalf("started=%+v", started)
	}
	server.flushBackgroundIO()

	q := httptest.NewRequest("GET", "/api/queue?session_id="+started.SessionID, nil)
	rec = httptest.NewRecorder()
	server.handleQueue(rec, q)
	if rec.Code != 200 {
		t.Fatalf("queue status=%d", rec.Code)
	}

	now := httptest.NewRequest("GET", "/api/now?session_id="+started.SessionID, nil)
	rec = httptest.NewRecorder()
	server.handleNow(rec, now)
	if rec.Code != 200 {
		t.Fatalf("now status=%d", rec.Code)
	}

	refresh := httptest.NewRequest("POST", "/api/queue/refresh?session_id="+started.SessionID, nil)
	rec = httptest.NewRecorder()
	server.handleQueueRefresh(rec, refresh)
	if rec.Code != 200 {
		t.Fatalf("refresh status=%d", rec.Code)
	}
	server.flushBackgroundIO()

	// like event
	evBody, _ := json.Marshal(map[string]any{
		"type": "like", "track_id": 11, "session_id": started.SessionID,
	})
	ev := httptest.NewRequest("POST", "/api/events", bytes.NewReader(evBody))
	rec = httptest.NewRecorder()
	server.handleEvents(rec, ev)
	if rec.Code != 200 {
		t.Fatalf("like status=%d body=%s", rec.Code, rec.Body.String())
	}

	// completed track_end should advance
	dur := 180.0
	listened := 170.0
	endBody, _ := json.Marshal(map[string]any{
		"type": "track_end", "track_id": 11, "session_id": started.SessionID,
		"duration_sec": dur, "listened_sec": listened, "reason": "completed",
	})
	ev = httptest.NewRequest("POST", "/api/events", bytes.NewReader(endBody))
	rec = httptest.NewRecorder()
	server.handleEvents(rec, ev)
	if rec.Code != 200 {
		t.Fatalf("track_end status=%d body=%s", rec.Code, rec.Body.String())
	}
	var ended struct {
		OK    bool  `json:"ok"`
		NextID int64 `json:"next_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ended); err != nil {
		t.Fatal(err)
	}
	if !ended.OK || ended.NextID == 0 {
		t.Fatalf("expected next track after end: %+v", ended)
	}
	server.flushBackgroundIO()

	var listens int
	if err := server.Store.DB.QueryRow(`SELECT COUNT(*) FROM listening_history`).Scan(&listens); err != nil {
		t.Fatal(err)
	}
	if listens < 1 {
		t.Fatal("events should write listening history")
	}
}

func TestEventsRequireSession(t *testing.T) {
	server := openTestServer(t)
	req := httptest.NewRequest("POST", "/api/events", bytes.NewReader([]byte(`{"type":"like","track_id":11}`)))
	rec := httptest.NewRecorder()
	server.handleEvents(rec, req)
	if rec.Code == 200 {
		t.Fatal("events without session should fail")
	}
}

func TestStatusIncludesSessionSnapshot(t *testing.T) {
	server := openTestServer(t)
	req := httptest.NewRequest("POST", "/api/radio/start", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	server.handleRadioStart(rec, req)
	var started struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	st := httptest.NewRequest("GET", "/api/status?session_id="+started.SessionID, nil)
	rec = httptest.NewRecorder()
	server.handleStatus(rec, st)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["session_id"] != started.SessionID {
		t.Fatalf("status session snapshot missing: %#v", body)
	}
}
