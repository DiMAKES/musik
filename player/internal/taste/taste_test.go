package taste

import (
	"math"
	"testing"
	"time"
)

func TestMaturity(t *testing.T) {
	p := New()
	p.SetCounts(0, 0)
	if p.Maturity(5, 15) != StatusDiscovering {
		t.Fatal("expected discovering")
	}
	p.SetCounts(5, 0)
	if p.Maturity(5, 15) != StatusForming {
		t.Fatal("expected forming")
	}
	p.SetCounts(15, 2)
	if p.Maturity(5, 15) != StatusReady {
		t.Fatal("expected ready")
	}
}

func TestWeightSkip(t *testing.T) {
	w, a := WeightFromListen(10, 200, "skipped")
	if w >= 0 || a != "skip" {
		t.Fatalf("got %v %s", w, a)
	}
	w, a = WeightFromListen(180, 200, "completed")
	if w < 0.8 || a != "finish" {
		t.Fatalf("completed weight=%v action=%s", w, a)
	}
}

func TestEffectiveExplore(t *testing.T) {
	p := New()
	p.SetCounts(0, 0)
	e := p.EffectiveExplore(0.25, 0.55, 5, 15)
	if e < 0.5 {
		t.Fatalf("discover explore too low: %v", e)
	}
	p.SetCounts(20, 0)
	e = p.EffectiveExplore(0.25, 0.55, 5, 15)
	if e != 0.25 {
		t.Fatalf("ready explore want 0.25 got %v", e)
	}
}

func TestUpdateEMAPositiveAndNegative(t *testing.T) {
	p := New()
	p.UpdateEMA([]float32{1, 0}, LikeWeight(), 0.5)
	if !p.Ready() {
		t.Fatal("taste should be ready after first update")
	}
	pos, neg := p.Counts()
	if pos != 1 || neg != 0 {
		t.Fatalf("counts=%d/%d", pos, neg)
	}
	before := append([]float32(nil), p.Get()...)
	p.UpdateEMA([]float32{0, 1}, DislikeWeight(), 0.5)
	pos, neg = p.Counts()
	if pos != 1 || neg != 1 {
		t.Fatalf("after dislike counts=%d/%d", pos, neg)
	}
	after := p.Get()
	if after[0] == before[0] && after[1] == before[1] {
		t.Fatal("EMA should change vector")
	}
	if p.SourceName() != "online_ema" {
		t.Fatalf("source=%q", p.SourceName())
	}
}

func TestThreeTasteVectorsAndNegativeBoundaries(t *testing.T) {
	long := New()
	state := NewState(long)
	session := SessionState{}
	state.UpdatePositive([]float32{1, 0}, 1, 0.5, "morning", &session)
	before := long.Get()

	state.AddEarlySkip(&session, []float32{0, 1}, 1)
	state.AddDislike([]float32{-1, 0}, 2)
	after := long.Get()
	if len(before) != len(after) || before[0] != after[0] || before[1] != after[1] {
		t.Fatalf("negative feedback changed long positive vector: %v -> %v", before, after)
	}
	if len(session.Negative) != 1 || len(state.PersistentNegatives()) != 1 {
		t.Fatalf("session/persistent negatives = %d/%d",
			len(session.Negative), len(state.PersistentNegatives()))
	}

	// One morning sample is below the daypart threshold, so an opposing sparse
	// daypart must not drag the effective direction away from long/session.
	state.RestoreDaypart("morning", VectorState{Vector: []float32{-1, 0}, Samples: 1})
	effective := state.Effective("morning", &session, 5)
	if effective[0] < 0.9 {
		t.Fatalf("sparse daypart did not fall back to long/session: %v", effective)
	}
}

func TestWeightedSphericalCentroids(t *testing.T) {
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	var samples []Sample
	for i := 0; i < 12; i++ {
		samples = append(samples, Sample{
			TrackID: int64(i + 1), Vector: []float32{1, 0},
			Weight: 1, At: now,
		})
		samples = append(samples, Sample{
			TrackID: int64(i + 101), Vector: []float32{0, 1},
			Weight: 1, At: now,
		})
	}
	// A repeated track has huge raw mass but is capped and cannot erase the
	// second mode.
	for i := 0; i < 20; i++ {
		samples = append(samples, Sample{
			TrackID: 1, Vector: []float32{1, 0}, Weight: 10, At: now,
		})
	}
	centroids := WeightedSphericalCentroids(samples, CentroidOptions{
		MinUniquePerCentroid: 10, MaxK: 6, PerTrackWeightCap: 2,
		Seed: 7, Now: now,
	})
	if len(centroids) != 2 {
		t.Fatalf("centroid count=%d, want 2: %+v", len(centroids), centroids)
	}
	for _, centroid := range centroids {
		norm := math.Sqrt(float64(
			centroid.Vector[0]*centroid.Vector[0] +
				centroid.Vector[1]*centroid.Vector[1],
		))
		if math.Abs(norm-1) > 1e-5 {
			t.Fatalf("centroid not spherical-normalized: %v", centroid.Vector)
		}
	}

	small := WeightedSphericalCentroids(samples[:6], CentroidOptions{
		MinUniquePerCentroid: 10, Seed: 7, Now: now,
	})
	if len(small) != 1 {
		t.Fatalf("small dataset produced %d centroids, want 1", len(small))
	}

	merged := WeightedSphericalCentroids([]Sample{
		{TrackID: 1, Vector: []float32{1, 0}, Weight: 1, At: now},
		{TrackID: 2, Vector: []float32{0.999, 0.01}, Weight: 1, At: now},
	}, CentroidOptions{MinUniquePerCentroid: 10, MergeSimilarity: 0.9, Seed: 3, Now: now})
	if len(merged) != 1 {
		t.Fatalf("near-duplicate centroids were not merged: %+v", merged)
	}

	warm := WeightedSphericalCentroids(samples, CentroidOptions{
		MinUniquePerCentroid: 10, MaxK: 6, Seed: 7, Now: now,
		WarmStart: []Centroid{{Vector: []float32{1, 0}}, {Vector: []float32{0, 1}}},
	})
	if len(warm) != 2 {
		t.Fatalf("warm start centroid count=%d, want 2", len(warm))
	}
}
