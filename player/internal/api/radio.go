package api

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) handleRadioStart(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	defer func() { s.latency.Observe("radio_start", time.Since(started)) }()
	var req struct {
		SeedTrackID *int64 `json:"seed_track_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	sess := s.newSession("radio")
	sess.mu.Lock()
	defer sess.mu.Unlock()
	s.seedRadioExcludeLocked(sess)
	startID := s.pickStartTrackLocked(sess, req.SeedTrackID)
	sess.Current = startID
	sess.Prev = 0
	s.excludeTrackLocked(sess, startID)
	s.refreshQueueFor(sess, startID, true)
	// refreshQueueFor already scheduleWarm
	writeJSON(w, map[string]any{
		"session_id": sess.ID,
		"mode":       "radio",
		"maturity":   s.maturityLocked(),
		"current":    s.trackJSON(startID),
		"queue":      sess.Queue,
	})
}
