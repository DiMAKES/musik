package queue

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/torwin-job/musik/player/internal/config"
	"github.com/torwin-job/musik/player/internal/db"
	"github.com/torwin-job/musik/player/internal/index"
)

func syntheticBuilder(b *testing.B, n, dim int) (*Builder, []float32) {
	b.Helper()
	rows := make([]db.TrackRow, n)
	rng := rand.New(rand.NewSource(1))
	for row := range rows {
		vec := make([]float32, dim)
		for d := range vec {
			vec[d] = rng.Float32()*2 - 1
		}
		index.Normalize(vec)
		rows[row] = db.TrackRow{
			ID: int64(row + 1), Path: fmt.Sprintf("/synthetic/%d.flac", row+1),
			Title: fmt.Sprintf("Track %d", row+1), Artist: fmt.Sprintf("Artist %d", row%500),
			Album: fmt.Sprintf("Album %d", row%2000), Duration: 180,
			Embedding: index.Float32Bytes(vec), Dim: dim, ClusterID: row % 32,
		}
	}
	cfg := config.Config{QueueSize: 6, ExploreRatio: 0.2}
	idx := index.New(cfg)
	if err := idx.Load(rows); err != nil {
		b.Fatal(err)
	}
	taste := idx.Vector(0)
	builder := NewBuilder(idx, cfg)
	builder.Rng = rand.New(rand.NewSource(1))
	return builder, taste
}

func BenchmarkExactQueueBuild(b *testing.B) {
	for _, n := range []int{2_000, 50_000} {
		b.Run(fmt.Sprintf("tracks_%d", n), func(b *testing.B) {
			builder, taste := syntheticBuilder(b, n, 512)
			exclude := map[int64]bool{1: true}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				items := builder.BuildCore(CoreOpts{CurrentID: 1, Taste: taste, Exclude: exclude, Size: 6})
				if len(items) == 0 {
					b.Fatal("empty queue")
				}
			}
		})
	}
}
