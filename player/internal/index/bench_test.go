package index

import (
	"fmt"
	"testing"

	"github.com/torwin-job/musik/player/internal/config"
)

func syntheticIndex(n, dim int) *Index {
	idx := New(config.Config{})
	idx.N, idx.D = n, dim
	idx.IDs = make([]int64, n)
	idx.Meta = make([]Meta, n)
	idx.Matrix = make([]float32, n*dim)
	idx.idRow = make(map[int64]int, n)
	state := uint64(1)
	for row := 0; row < n; row++ {
		idx.IDs[row] = int64(row + 1)
		idx.Meta[row] = Meta{ID: int64(row + 1)}
		idx.idRow[int64(row+1)] = row
		vec := idx.Matrix[row*dim : (row+1)*dim]
		for d := range vec {
			state = state*6364136223846793005 + 1442695040888963407
			vec[d] = float32(int32(state>>32)) / (1 << 31)
		}
		Normalize(vec)
	}
	return idx
}

func BenchmarkFusedExactScan(b *testing.B) {
	for _, n := range []int{2_000, 50_000} {
		b.Run(fmt.Sprintf("tracks_%d", n), func(b *testing.B) {
			idx := syntheticIndex(n, 512)
			queries := [][]float32{
				append([]float32(nil), idx.Matrix[:512]...),
				append([]float32(nil), idx.Matrix[512:1024]...),
				append([]float32(nil), idx.Matrix[1024:1536]...),
			}
			b.ReportAllocs()
			b.SetBytes(int64(n * 512 * 4))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				results := idx.FusedSimsTo(queries...)
				if len(results) != len(queries) {
					b.Fatal("missing fused results")
				}
			}
		})
	}
}
