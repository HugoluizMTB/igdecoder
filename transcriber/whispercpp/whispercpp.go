package whispercpp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hugoluizmtb/igdecoder"
)

type Config struct {
	Binary    string
	Model     string
	Language  string
	Threads   int
	ExtraArgs []string
}

type Transcriber struct {
	cfg Config
}

func New(cfg Config) *Transcriber {
	if cfg.Binary == "" {
		cfg.Binary = "whisper-cli"
	}
	if cfg.Language == "" {
		cfg.Language = "auto"
	}
	return &Transcriber{cfg: cfg}
}

func (t *Transcriber) Transcribe(ctx context.Context, audioPath string, hint igdecoder.TranscribeHint) (igdecoder.Transcript, error) {
	if t.cfg.Model == "" {
		return igdecoder.Transcript{}, fmt.Errorf("whispercpp: Config.Model vazio")
	}
	if _, err := os.Stat(t.cfg.Model); err != nil {
		return igdecoder.Transcript{}, fmt.Errorf("whispercpp: modelo não encontrado em %q: %w", t.cfg.Model, err)
	}

	lang := t.cfg.Language
	if (lang == "" || lang == "auto") && hint.Language != "" {
		lang = hint.Language
	}

	outBase := strings.TrimSuffix(audioPath, filepath.Ext(audioPath)) + ".whisper"
	outJSON := outBase + ".json"
	defer os.Remove(outJSON)

	args := []string{"-m", t.cfg.Model, "-f", audioPath, "-oj", "-of", outBase, "-l", lang, "-np"}
	if t.cfg.Threads > 0 {
		args = append(args, "-t", strconv.Itoa(t.cfg.Threads))
	}
	args = append(args, t.cfg.ExtraArgs...)

	var stderr strings.Builder
	cmd := exec.CommandContext(ctx, t.cfg.Binary, args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return igdecoder.Transcript{}, ctx.Err()
		}
		return igdecoder.Transcript{}, fmt.Errorf("whispercpp: %w: %s", err, tail(stderr.String(), 300))
	}

	raw, err := os.ReadFile(outJSON)
	if err != nil {
		return igdecoder.Transcript{}, fmt.Errorf("whispercpp: ler saída json: %w", err)
	}
	return parse(raw, t.cfg.Model)
}

type wjson struct {
	Result struct {
		Language string `json:"language"`
	} `json:"result"`
	Transcription []struct {
		Text    string `json:"text"`
		Offsets struct {
			From int64 `json:"from"`
			To   int64 `json:"to"`
		} `json:"offsets"`
	} `json:"transcription"`
}

func parse(raw []byte, model string) (igdecoder.Transcript, error) {
	var w wjson
	if err := json.Unmarshal(raw, &w); err != nil {
		return igdecoder.Transcript{}, fmt.Errorf("whispercpp: json inválido: %w", err)
	}

	var segs []igdecoder.Segment
	var sb strings.Builder
	for _, seg := range w.Transcription {
		txt := strings.TrimSpace(seg.Text)
		if txt == "" || isNoise(txt) {
			continue
		}
		segs = append(segs, igdecoder.Segment{
			Start: time.Duration(seg.Offsets.From) * time.Millisecond,
			End:   time.Duration(seg.Offsets.To) * time.Millisecond,
			Text:  txt,
		})
		if sb.Len() > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(txt)
	}

	text := strings.TrimSpace(sb.String())
	var dur time.Duration
	if n := len(w.Transcription); n > 0 {
		dur = time.Duration(w.Transcription[n-1].Offsets.To) * time.Millisecond
	}

	return igdecoder.Transcript{
		Text:     text,
		Language: w.Result.Language,
		Segments: segs,
		Engine:   "whisper.cpp",
		Model:    filepath.Base(model),
		Duration: dur,
		NoSpeech: text == "",
	}, nil
}

func isNoise(s string) bool {
	s = strings.ToLower(strings.Trim(s, " []()*_-"))
	switch s {
	case "blank_audio", "silence", "silêncio", "music", "música", "inaudible", "inaudível", "sound", "applause", "aplausos":
		return true
	}
	return false
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
