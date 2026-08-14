package igdecoder

import (
	"context"
	"errors"
	"fmt"
	"os"
)

var ErrNilTranscriber = errors.New("igdecoder: transcriber nil")

func (c *Client) Transcribe(ctx context.Context, m Media, tr Transcriber) (Transcript, error) {
	if tr == nil {
		return Transcript{}, ErrNilTranscriber
	}
	if !m.IsVideo() {
		return Transcript{}, ErrNoMedia
	}
	if c.maxDuration > 0 && m.Duration > c.maxDuration {
		return Transcript{}, fmt.Errorf("igdecoder: mídia %s dura %s, acima do limite %s",
			m.ID, m.Duration, c.maxDuration)
	}

	dir, err := os.MkdirTemp(c.tempDir(), "igdec-")
	if err != nil {
		return Transcript{}, fmt.Errorf("igdecoder: transcribe: tempdir: %w", err)
	}
	defer os.RemoveAll(dir)

	mediaPath, err := c.Download(ctx, m, dir)
	if err != nil {
		return Transcript{}, err
	}

	wavPath, err := c.extractWAV(ctx, mediaPath)
	if err != nil {
		return Transcript{}, err
	}

	hint := TranscribeHint{Duration: m.Duration, MediaID: m.ID}
	t, err := tr.Transcribe(ctx, wavPath, hint)
	if err != nil {
		return Transcript{}, fmt.Errorf("igdecoder: transcribe: %w", err)
	}
	if t.Duration == 0 {
		t.Duration = m.Duration
	}
	t.NoSpeech = t.NoSpeech || len(t.Segments) == 0 && t.Text == ""
	return t, nil
}

func (c *Client) TranscribeFile(ctx context.Context, path string, tr Transcriber) (Transcript, error) {
	if tr == nil {
		return Transcript{}, ErrNilTranscriber
	}
	wavPath, err := c.extractWAV(ctx, path)
	if err != nil {
		return Transcript{}, err
	}
	defer os.Remove(wavPath)
	return tr.Transcribe(ctx, wavPath, TranscribeHint{})
}
