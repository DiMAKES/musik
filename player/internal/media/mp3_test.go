package media

import (
	"context"
	"testing"
	"time"
)

func TestMP3BytesPerSec(t *testing.T) {
	if got := mp3BytesPerSec("192k"); got != 24000 {
		t.Fatalf("192k = %v", got)
	}
	if got := mp3BytesPerSec("128k"); got != 16000 {
		t.Fatalf("128k = %v", got)
	}
	if got := mp3BytesPerSec(""); got != 24000 {
		t.Fatalf("empty = %v", got)
	}
}

func TestLivePacerBurstThenWaits(t *testing.T) {
	p := newLivePacer(1000)
	ctx := context.Background()
	start := time.Now()
	if err := p.Wait(ctx, p.burst); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 80*time.Millisecond {
		t.Fatalf("burst waited %s", time.Since(start))
	}
	if err := p.Wait(ctx, 400); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 300*time.Millisecond {
		t.Fatalf("bytes past the burst were sent immediately (%s)", elapsed)
	}
}

func TestLivePacerCancelled(t *testing.T) {
	p := newLivePacer(1000)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.Wait(ctx, p.burst+5000)
	if err == nil {
		t.Fatal("expected cancel")
	}
}
