package media

import "testing"

func TestSnapArtworkWidth(t *testing.T) {
	tests := []struct {
		in, want int
	}{
		{1, 96},
		{64, 96},
		{96, 96},
		{176, 256},
		{200, 256},
		{256, 256},
		{400, 256},
		{448, 640},
		{500, 640},
		{640, 640},
		{2000, 640},
	}
	for _, tc := range tests {
		if got := snapArtworkWidth(tc.in); got != tc.want {
			t.Errorf("snapArtworkWidth(%d)=%d, want %d", tc.in, got, tc.want)
		}
	}
}
