package igdecoder

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func newTestClient() *Client {
	return &Client{ffmpegBin: "ffmpeg", log: slog.Default()}
}

func TestExtractWAVIntegration(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg ausente")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "in.mp4")
	gen := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-f", "lavfi", "-i", "color=c=black:s=64x64:d=1",
		"-shortest", "-pix_fmt", "yuv420p", src)
	if err := gen.Run(); err != nil {
		t.Skipf("ffmpeg não gerou fixture: %v", err)
	}

	wav, err := newTestClient().extractWAV(context.Background(), src)
	if err != nil {
		t.Fatalf("extractWAV: %v", err)
	}
	defer os.Remove(wav)
	fi, err := os.Stat(wav)
	if err != nil {
		t.Fatalf("stat wav: %v", err)
	}
	if fi.Size() < 1024 {
		t.Fatalf("wav pequeno demais: %d bytes", fi.Size())
	}
}

func TestExtractWAVNoAudio(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg ausente")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "silent.mp4")
	gen := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=64x64:d=1",
		"-pix_fmt", "yuv420p", src)
	if err := gen.Run(); err != nil {
		t.Skipf("ffmpeg não gerou fixture: %v", err)
	}

	_, err := newTestClient().extractWAV(context.Background(), src)
	if !errors.Is(err, ErrNoMedia) {
		t.Fatalf("esperava ErrNoMedia para vídeo sem áudio, veio: %v", err)
	}
}
