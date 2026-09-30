package playback

import (
	"time"

	"github.com/torwin-job/musik/player/internal/db"
	"github.com/torwin-job/musik/player/internal/index"
	"github.com/torwin-job/musik/player/internal/rules"
	"github.com/torwin-job/musik/player/internal/taste"
)

func (e *Engine) SetSessionContexts(sess *Session, ids []string) error {
	sess.ActiveContextIDs = append([]string(nil), ids...)
	if err := e.Store.SetSessionContexts(sess.ID, ids); err != nil {
		return err
	}
	e.persistLocked(sess)
	return nil
}

func (e *Engine) contextBlends(ids []string) []taste.ContextBlend {
	var out []taste.ContextBlend
	for _, id := range ids {
		ctx, err := e.Store.GetTasteContext(id)
		if err != nil || ctx == nil || ctx.ArchivedAt != "" {
			continue
		}
		state, _ := e.Store.LoadTasteContextState(id)
		var vector []float32
		if state != nil && len(state.PositiveVector) > 0 {
			vector = index.BytesToFloat32(state.PositiveVector)
		} else {
			albums := make([][2]string, 0, len(ctx.Seeds.Albums))
			for _, album := range ctx.Seeds.Albums {
				albums = append(albums, [2]string{album.Artist, album.Album})
			}
			vector = taste.SeedVector(e.Idx, ctx.Seeds.Tracks, ctx.Seeds.Artists, albums)
		}
		if len(vector) == 0 {
			continue
		}
		samples := 0
		if state != nil {
			samples = state.PositiveSamples
		}
		out = append(out, taste.ContextBlend{
			ID: ctx.ID, Kind: ctx.Kind, Influence: ctx.Influence,
			Vector: vector, Samples: samples,
		})
	}
	return out
}

func (e *Engine) sessionRules(sess *Session) *rules.Evaluator {
	active, err := e.Store.ActiveRadioRules(sess.ID, sess.ActiveContextIDs)
	if err != nil || len(active) == 0 {
		return nil
	}
	return rules.New(e.Idx, active, time.Now().UTC())
}

func (e *Engine) applyHardBlocks(sess *Session) {
	eval := e.sessionRules(sess)
	if eval == nil {
		return
	}
	n := e.Idx.Size()
	for i := 0; i < n; i++ {
		id := e.Idx.MetaAt(i).ID
		if eval.HardBlocked(id) {
			e.ExcludeTrack(sess, id)
		}
	}
}

func (e *Engine) learnActiveContexts(sess *Session, vector []float32, weight float64) {
	if weight <= 0 || len(vector) == 0 {
		return
	}
	for _, id := range sess.ActiveContextIDs {
		ctx, err := e.Store.GetTasteContext(id)
		if err != nil || ctx == nil || !ctx.LearningEnabled {
			continue
		}
		state, _ := e.Store.LoadTasteContextState(id)
		current := taste.VectorState{}
		if state != nil {
			current.Vector = index.BytesToFloat32(state.PositiveVector)
			current.Samples = state.PositiveSamples
		}
		updated := taste.ApplyPositive(current, vector, weight, e.Cfg.TasteAlpha)
		_ = e.Store.UpsertTasteContextState(db.TasteContextState{
			ContextID: id, PositiveVector: index.Float32Bytes(updated.Vector),
			EmbeddingDim: len(updated.Vector), PositiveSamples: updated.Samples,
		})
	}
}
