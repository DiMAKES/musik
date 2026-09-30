package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/torwin-job/musik/player/internal/db"
	"github.com/torwin-job/musik/player/internal/index"
	"github.com/torwin-job/musik/player/internal/taste"
)

type contextWrite struct {
	Kind            string          `json:"kind"`
	Name            string          `json:"name"`
	Icon            string          `json:"icon"`
	Influence       float64         `json:"influence"`
	LearningEnabled *bool           `json:"learning_enabled"`
	Seeds           db.ContextSeeds `json:"seeds"`
}

func (s *Server) handleContextsList(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListTasteContexts(r.URL.Query().Get("all") == "1")
	if err != nil {
		writeErr(w, 500, "db", err.Error())
		return
	}
	if list == nil {
		list = []db.TasteContext{}
	}
	writeJSON(w, map[string]any{"contexts": list})
}

func (s *Server) handleContextsCreate(w http.ResponseWriter, r *http.Request) {
	req, err := decodeContextWrite(r)
	if err != nil {
		writeErr(w, 400, "bad_json", "bad json")
		return
	}
	ctx := db.TasteContext{
		Kind: req.Kind, Name: req.Name, Icon: req.Icon,
		Influence: req.Influence, Seeds: req.Seeds, LearningEnabled: true,
	}
	if req.LearningEnabled != nil {
		ctx.LearningEnabled = *req.LearningEnabled
	}
	created, err := s.Store.CreateTasteContext(ctx)
	if err != nil {
		writeErr(w, 400, "context", err.Error())
		return
	}
	s.seedContextVector(created)
	out, _ := s.Store.GetTasteContext(created.ID)
	writeJSON(w, out)
}

func (s *Server) handleContextGet(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.Store.GetTasteContext(r.PathValue("id"))
	if err != nil {
		writeErr(w, 500, "db", err.Error())
		return
	}
	if ctx == nil {
		writeErr(w, 404, "not_found", "context not found")
		return
	}
	writeJSON(w, ctx)
}

func (s *Server) handleContextPatch(w http.ResponseWriter, r *http.Request) {
	req, err := decodeContextWrite(r)
	if err != nil {
		writeErr(w, 400, "bad_json", "bad json")
		return
	}
	existing, err := s.Store.GetTasteContext(r.PathValue("id"))
	if err != nil || existing == nil {
		writeErr(w, 404, "not_found", "context not found")
		return
	}
	existing.Name = firstNonEmpty(req.Name, existing.Name)
	existing.Icon = firstNonEmpty(req.Icon, existing.Icon)
	if req.Influence > 0 {
		existing.Influence = req.Influence
	}
	if req.LearningEnabled != nil {
		existing.LearningEnabled = *req.LearningEnabled
	}
	if req.Seeds.SchemaVersion > 0 || len(req.Seeds.Tracks)+len(req.Seeds.Artists)+len(req.Seeds.Albums) > 0 {
		existing.Seeds = req.Seeds
		if existing.Seeds.SchemaVersion == 0 {
			existing.Seeds.SchemaVersion = 1
		}
	}
	if err := s.Store.UpdateTasteContext(*existing); err != nil {
		writeErr(w, 400, "context", err.Error())
		return
	}
	s.seedContextVector(*existing)
	out, _ := s.Store.GetTasteContext(existing.ID)
	writeJSON(w, out)
}

func (s *Server) handleContextDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.ArchiveTasteContext(r.PathValue("id")); err != nil {
		writeErr(w, 500, "db", err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleContextActivate(w http.ResponseWriter, r *http.Request) {
	s.toggleContext(w, r, true)
}

func (s *Server) handleContextDeactivate(w http.ResponseWriter, r *http.Request) {
	s.toggleContext(w, r, false)
}

func (s *Server) toggleContext(w http.ResponseWriter, r *http.Request, on bool) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	sess := s.Play.Get(req.SessionID)
	if sess == nil {
		writeErr(w, 404, "not_found", "session not found")
		return
	}
	id := r.PathValue("id")
	sess.Lock()
	defer sess.Unlock()
	next := make([]string, 0, len(sess.ActiveContextIDs)+1)
	have := false
	for _, existing := range sess.ActiveContextIDs {
		if existing == id {
			have = true
			if on {
				next = append(next, existing)
			}
			continue
		}
		next = append(next, existing)
	}
	if on && !have {
		next = append(next, id)
	}
	if err := s.Play.SetSessionContexts(sess, next); err != nil {
		writeErr(w, 500, "db", err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "context_ids": next})
}

func (s *Server) handleSessionContexts(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID  string   `json:"session_id"`
		ContextIDs []string `json:"context_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "bad_json", "bad json")
		return
	}
	sess := s.Play.Get(req.SessionID)
	if sess == nil {
		writeErr(w, 404, "not_found", "session not found")
		return
	}
	sess.Lock()
	defer sess.Unlock()
	if err := s.Play.SetSessionContexts(sess, req.ContextIDs); err != nil {
		writeErr(w, 500, "db", err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "context_ids": req.ContextIDs})
}

func (s *Server) seedContextVector(ctx db.TasteContext) {
	albums := make([][2]string, 0, len(ctx.Seeds.Albums))
	for _, album := range ctx.Seeds.Albums {
		albums = append(albums, [2]string{album.Artist, album.Album})
	}
	vec := taste.SeedVector(s.Idx, ctx.Seeds.Tracks, ctx.Seeds.Artists, albums)
	if len(vec) == 0 {
		return
	}
	samples := 0
	if state, _ := s.Store.LoadTasteContextState(ctx.ID); state != nil {
		samples = state.PositiveSamples
	}
	_ = s.Store.UpsertTasteContextState(db.TasteContextState{
		ContextID: ctx.ID, PositiveVector: index.Float32Bytes(vec),
		EmbeddingDim: len(vec), PositiveSamples: samples,
	})
}

func decodeContextWrite(r *http.Request) (contextWrite, error) {
	var req contextWrite
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return req, err
	}
	if len(body) == 0 {
		return req, nil
	}
	return req, json.Unmarshal(body, &req)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
