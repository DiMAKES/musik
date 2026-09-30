package taste

import "testing"

func TestBlendContextsIndependentAndZeroWhenMissing(t *testing.T) {
	base := []float32{1, 0}
	mood := ContextBlend{Kind: "mood", Influence: 1, Vector: []float32{0, 1}}
	out := BlendContexts(base, []ContextBlend{mood})
	if len(out) != 2 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0] <= 0 || out[1] <= 0 {
		t.Fatalf("expected both axes after blend: %v", out)
	}
	empty := BlendContexts(base, []ContextBlend{{Kind: "place", Influence: 1}})
	if empty[0] < 0.99 {
		t.Fatalf("missing context should not move the vector: %v", empty)
	}
	sims := ContextSimilarities([]float32{0, 1}, []ContextBlend{mood})
	if sims["mood"] < 0.99 {
		t.Fatalf("mood sim=%v", sims)
	}
}
