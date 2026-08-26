package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/torwin-job/musik/player/internal/config"
	"github.com/torwin-job/musik/player/internal/db"
	"github.com/torwin-job/musik/player/internal/index"
	"github.com/torwin-job/musik/player/internal/taste"
)

func openTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "db", "musik-api-test.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE tracks (
			id INTEGER PRIMARY KEY, path TEXT NOT NULL DEFAULT '',
			title TEXT, artist TEXT, album TEXT, duration REAL
		)`,
		`CREATE TABLE listening_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT, track_id INTEGER NOT NULL,
			ts TEXT NOT NULL, source TEXT, action TEXT NOT NULL,
			daypart TEXT, weekday INTEGER, position_sec REAL, duration_sec REAL
		)`,
		`CREATE TABLE playlists (
			id INTEGER PRIMARY KEY, kind TEXT NOT NULL, name TEXT NOT NULL, created_at TEXT NOT NULL
		)`,
		`CREATE TABLE playlist_tracks (
			playlist_id INTEGER NOT NULL, position INTEGER NOT NULL,
			track_id INTEGER NOT NULL, explanation TEXT
		)`,
	} {
		if _, err := bootstrap.Exec(statement); err != nil {
			_ = bootstrap.Close()
			t.Fatalf("create test schema: %v", err)
		}
	}
	if err := bootstrap.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := config.Config{
		DBPath: path, QueueSize: 6, AuthDisabled: true,
		ProfileFormingAt: 3, ProfileReadyAt: 8, ExploreRatio: 0.15,
		DiscoverExploreRatio: 0.35, WorkerURL: "http://127.0.0.1:1",
	}
	idx := index.New(cfg)
	rows := make([]db.TrackRow, 0, 3)
	for _, id := range []int64{11, 22, 33} {
		rows = append(rows, db.TrackRow{
			ID: id, Path: filepath.Join(dir, "missing.flac"),
			Title: "Track", Artist: "Artist", Album: "Album", Duration: 180,
			Embedding: index.Float32Bytes([]float32{1, float32(id)}), Dim: 2,
		})
		if _, err := store.DB.Exec(
			`INSERT INTO tracks(id, path, title, artist, album, duration) VALUES (?,?,?,?,?,?)`,
			id, filepath.Join(dir, "missing.flac"), "Track", "Artist", "Album", 180,
		); err != nil {
			t.Fatal(err)
		}
	}
	if err := idx.Load(rows); err != nil {
		t.Fatal(err)
	}
	server := New(cfg, store, idx, taste.New(), nil)
	server.Warm = nil
	return server
}

func TestWeekdayKind(t *testing.T) {
	tests := []struct {
		day  time.Weekday
		want string
	}{
		{time.Monday, "weekday_mon"},
		{time.Tuesday, "weekday_tue"},
		{time.Wednesday, "weekday_wed"},
		{time.Thursday, "weekday_thu"},
		{time.Friday, "weekday_fri"},
		{time.Saturday, "weekday_sat"},
		{time.Sunday, "weekday_sun"},
	}
	for _, test := range tests {
		if got := weekdayKind(test.day); got != test.want {
			t.Errorf("weekdayKind(%s) = %q, want %q", test.day, got, test.want)
		}
	}
}

func TestFixedSessionPreservesOrderAndStartPosition(t *testing.T) {
	tests := []struct {
		name         string
		startIndex   int
		startTrackID int64
		wantIndex    int
		wantCurrent  int64
	}{
		{name: "index", startIndex: 2, wantIndex: 2, wantCurrent: 33},
		{name: "track id overrides index", startIndex: 0, startTrackID: 22, wantIndex: 1, wantCurrent: 22},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := openTestServer(t)
			order := []int64{11, 22, 33}
			session := server.startFixedLocked(
				append([]int64(nil), order...), "playlist", "Ordered", "daily",
				test.startIndex, test.startTrackID,
			)
			if !reflect.DeepEqual(session.DailyIDs, order) {
				t.Fatalf("session order = %v, want %v", session.DailyIDs, order)
			}
			if session.DailyPos != test.wantIndex || session.Current != test.wantCurrent {
				t.Fatalf("position/current = %d/%d, want %d/%d",
					session.DailyPos, session.Current, test.wantIndex, test.wantCurrent)
			}

			row, ok, err := server.Store.LoadPlaySession(session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatal("fixed session was not persisted")
			}
			var persistedOrder []int64
			if err := json.Unmarshal([]byte(row.DailyIDsJSON), &persistedOrder); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(persistedOrder, order) {
				t.Fatalf("persisted order = %v, want %v", persistedOrder, order)
			}
			if row.DailyPos != test.wantIndex || row.CurrentID != test.wantCurrent {
				t.Fatalf("persisted position/current = %d/%d, want %d/%d",
					row.DailyPos, row.CurrentID, test.wantIndex, test.wantCurrent)
			}
		})
	}
}

func TestIndependentRadioSessionsRunConcurrently(t *testing.T) {
	server := openTestServer(t)
	const clients = 8
	start := make(chan struct{})
	errs := make(chan string, clients)
	ids := make(chan string, clients)
	var wg sync.WaitGroup
	wg.Add(clients)
	for i := 0; i < clients; i++ {
		go func() {
			defer wg.Done()
			<-start
			req := httptest.NewRequest("POST", "/api/radio/start", bytes.NewReader([]byte(`{}`)))
			rec := httptest.NewRecorder()
			server.handleRadioStart(rec, req)
			if rec.Code != 200 {
				errs <- rec.Body.String()
				return
			}
			var body struct {
				SessionID string `json:"session_id"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				errs <- err.Error()
				return
			}
			ids <- body.SessionID
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	close(ids)
	for err := range errs {
		t.Error(err)
	}
	seen := map[string]bool{}
	for id := range ids {
		if id == "" || seen[id] {
			t.Fatalf("bad or duplicate session id %q", id)
		}
		seen[id] = true
	}
	if len(seen) != clients {
		t.Fatalf("sessions=%d, want %d", len(seen), clients)
	}
	server.flushBackgroundIO()
}
