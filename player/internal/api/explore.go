package api

import (
	"encoding/json"
	"net/http"

	"github.com/torwin-job/musik/player/internal/queue"
)

func (s *Server) exploreState() map[string]any {
	prefs, _ := s.Store.LoadRadioPrefs()
	lo, hi := queue.ClampExploreBounds(prefs.ExploreLo, prefs.ExploreHi)
	if prefs.ExploreLo == 0 && prefs.ExploreHi == 0 {
		lo, hi = queue.ClampExploreBounds(s.Cfg.ExploreLo, s.Cfg.ExploreHi)
	}
	counts, _ := s.Store.ExploreSourceOutcomeCounts()
	if counts == nil {
		counts = map[string]int{}
	}
	ready := queue.ExploreReady(counts)
	arms, _ := s.Store.LoadExploreArms()
	armOut := make([]map[string]any, 0, len(arms))
	for _, arm := range arms {
		total := arm.Alpha + arm.Beta
		mean := 0.5
		if total > 0 {
			mean = arm.Alpha / total
		}
		armOut = append(armOut, map[string]any{
			"key": arm.ArmKey, "kind": arm.ArmKind,
			"alpha": arm.Alpha, "beta": arm.Beta,
			"successes": arm.Successes, "failures": arm.Failures,
			"mean": mean,
		})
	}
	return map[string]any{
		"explore_lo":          lo,
		"explore_hi":          hi,
		"explore_ratio":       s.Play.Explore(),
		"bandit_ready":        ready,
		"source_outcomes":     counts,
		"min_source_outcomes": queue.MinSourceOutcomes,
		"arms":                armOut,
	}
}

func (s *Server) handleExploreBounds(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExploreLo *float64 `json:"explore_lo"`
		ExploreHi *float64 `json:"explore_hi"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "bad_json", "invalid json")
		return
	}
	prefs, _ := s.Store.LoadRadioPrefs()
	lo, hi := prefs.ExploreLo, prefs.ExploreHi
	if lo == 0 && hi == 0 {
		lo, hi = s.Cfg.ExploreLo, s.Cfg.ExploreHi
	}
	if body.ExploreLo != nil {
		lo = *body.ExploreLo
	}
	if body.ExploreHi != nil {
		hi = *body.ExploreHi
	}
	lo, hi = queue.ClampExploreBounds(lo, hi)
	if err := s.Store.SaveRadioPrefs(lo, hi); err != nil {
		writeErr(w, 500, "db", err.Error())
		return
	}
	writeJSON(w, s.exploreState())
}
