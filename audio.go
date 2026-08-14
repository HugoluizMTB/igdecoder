package igdecoder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrNoAudioTrack = fmt.Errorf("%w: sem faixa de áudio", ErrNoMedia)

func (c *Client) extractWAV(ctx context.Context, mediaPath string) (string, error) {
	wavPath := strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath)) + ".16k.wav"

	args := []string{
		"-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-i", mediaPath,
		"-vn", "-ac", "1", "-ar", "16000",
		"-c:a", "pcm_s16le", "-f", "wav",
		wavPath,
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.ffmpegBin, args...)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if noAudioStream(msg) {
			return "", ErrNoAudioTrack
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", ctx.Err()
		}
		if isExecNotFound(err) {
			return "", fmt.Errorf("igdecoder: ffmpeg não encontrado (%q): instale o ffmpeg ou use WithFFmpegPath: %w",
				c.ffmpegBin, err)
		}
		return "", fmt.Errorf("igdecoder: ffmpeg: %w: %s", err, truncate([]byte(msg), 300))
	}

	if fi, err := os.Stat(wavPath); err == nil && fi.Size() < 1024 {
		_ = os.Remove(wavPath)
		return "", ErrNoAudioTrack
	}
	return wavPath, nil
}

func noAudioStream(ffmpegErr string) bool {
	e := strings.ToLower(ffmpegErr)
	return strings.Contains(e, "does not contain any stream") ||
		strings.Contains(e, "output file #0 does not contain any stream") ||
		(strings.Contains(e, "stream map") && strings.Contains(e, "matches no streams"))
}

func isExecNotFound(err error) bool {
	return errors.Is(err, exec.ErrNotFound) ||
		strings.Contains(err.Error(), "executable file not found")
}
