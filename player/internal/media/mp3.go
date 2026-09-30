package media

import (
	"context"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Flusher flushes buffered stream data to a listener.
type Flusher interface {
	Flush()
}

// liveBurst is how far ahead of realtime a listener may buffer.
// Enough to start playback, not enough to skip forward through the show.
const liveBurst = 4 * time.Second

// PipeTrackMP3 transcodes a track to a live MP3 stream.
// Output is paced to the nominal bitrate and has no duration header, so a
// player cannot download the track ahead of playback and seek forward.
func PipeTrackMP3(
	ctx context.Context,
	w io.Writer,
	flusher Flusher,
	ffmpeg, path, bitrate string,
) error {
	cmd := exec.CommandContext(ctx, ffmpeg,
		"-nostdin", "-hide_banner", "-loglevel", "error",
		"-re",
		"-i", path,
		"-vn",
		"-map_metadata", "-1",
		"-acodec", "libmp3lame",
		"-b:a", bitrate,
		"-ar", "44100",
		"-ac", "2",
		"-f", "mp3",
		"-write_xing", "0",
		"-id3v2_version", "0",
		"pipe:1",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}
	pacer := newLivePacer(mp3BytesPerSec(bitrate))
	buf := make([]byte, 16*1024)
	for {
		if ctx.Err() != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return ctx.Err()
		}
		n, readErr := stdout.Read(buf)
		if n > 0 {
			if err := pacer.Wait(ctx, n); err != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				return err
			}
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				return writeErr
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			waitErr := cmd.Wait()
			if readErr == io.EOF {
				return waitErr
			}
			return readErr
		}
	}
}

// mp3BytesPerSec parses an ffmpeg bitrate like "192k" into bytes per second.
func mp3BytesPerSec(bitrate string) float64 {
	s := strings.TrimSpace(strings.ToLower(bitrate))
	s = strings.TrimSuffix(s, "bps")
	mult := 1.0
	switch {
	case strings.HasSuffix(s, "k"):
		mult = 1000
		s = strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "m"):
		mult = 1000 * 1000
		s = strings.TrimSuffix(s, "m")
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n <= 0 {
		n = 192
		mult = 1000
	}
	return n * mult / 8
}

// livePacer holds a listener to the nominal bitrate after a short start burst.
type livePacer struct {
	bytesPerSec float64
	start       time.Time
	sent        int
	burst       int
}

func newLivePacer(bytesPerSec float64) *livePacer {
	if bytesPerSec < 1000 {
		bytesPerSec = 192000 / 8
	}
	return &livePacer{
		bytesPerSec: bytesPerSec,
		start:       time.Now(),
		burst:       int(bytesPerSec * liveBurst.Seconds()),
	}
}

// Wait blocks until n more bytes are within the live budget.
func (p *livePacer) Wait(ctx context.Context, n int) error {
	if n <= 0 {
		return nil
	}
	p.sent += n
	allowed := p.burst + int(time.Since(p.start).Seconds()*p.bytesPerSec)
	extra := p.sent - allowed
	if extra <= 0 {
		return nil
	}
	delay := time.Duration(float64(extra) / p.bytesPerSec * float64(time.Second))
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
