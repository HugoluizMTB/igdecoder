package whispercpp

import (
	"context"
	"testing"
	"time"

	"github.com/hugoluizmtb/igdecoder"
)

func TestParseBuildsTranscript(t *testing.T) {
	raw := []byte(`{
		"result": {"language": "pt"},
		"transcription": [
			{"text": " Olá pessoal", "offsets": {"from": 0, "to": 1500}},
			{"text": " tudo bem?", "offsets": {"from": 1500, "to": 3000}}
		]
	}`)
	got, err := parse(raw, "/models/ggml-large-v3-turbo.bin")
	if err != nil {
		t.Fatal(err)
	}
	if got.Language != "pt" {
		t.Errorf("language = %q", got.Language)
	}
	if got.Text != "Olá pessoal tudo bem?" {
		t.Errorf("text = %q", got.Text)
	}
	if len(got.Segments) != 2 {
		t.Fatalf("segments = %d", len(got.Segments))
	}
	if got.Segments[0].Start != 0 || got.Segments[0].End != 1500*time.Millisecond {
		t.Errorf("offsets = %v..%v", got.Segments[0].Start, got.Segments[0].End)
	}
	if got.Duration != 3*time.Second {
		t.Errorf("duration = %v", got.Duration)
	}
	if got.Engine != "whisper.cpp" {
		t.Errorf("engine = %q", got.Engine)
	}
	if got.Model != "ggml-large-v3-turbo.bin" {
		t.Errorf("model = %q", got.Model)
	}
	if got.NoSpeech {
		t.Error("NoSpeech deveria ser false")
	}
}

func TestParseFiltersNoise(t *testing.T) {
	raw := []byte(`{
		"result": {"language": "pt"},
		"transcription": [
			{"text": "[BLANK_AUDIO]", "offsets": {"from": 0, "to": 1000}},
			{"text": " (música)", "offsets": {"from": 1000, "to": 2000}},
			{"text": "  ", "offsets": {"from": 2000, "to": 2500}}
		]
	}`)
	got, err := parse(raw, "m.bin")
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "" {
		t.Errorf("ruído deveria ser filtrado, veio %q", got.Text)
	}
	if !got.NoSpeech {
		t.Error("NoSpeech deveria ser true")
	}
	if len(got.Segments) != 0 {
		t.Errorf("segments = %d, queria 0", len(got.Segments))
	}
}

func TestParseInvalidJSON(t *testing.T) {
	if _, err := parse([]byte(`nao é json`), "m.bin"); err == nil {
		t.Error("json inválido deveria falhar")
	}
}

func TestIsNoise(t *testing.T) {
	for _, s := range []string{"[BLANK_AUDIO]", "(música)", "*applause*", "SILENCE"} {
		if !isNoise(s) {
			t.Errorf("%q deveria ser ruído", s)
		}
	}
	for _, s := range []string{"olá", "a música tocou"} {
		if isNoise(s) {
			t.Errorf("%q não é ruído", s)
		}
	}
}

func TestTranscribeRequiresModel(t *testing.T) {
	tr := New(Config{})
	_, err := tr.Transcribe(context.Background(), "x.wav", igdecoder.TranscribeHint{})
	if err == nil {
		t.Error("modelo vazio deveria falhar")
	}
}

func TestTranscribeMissingModelFile(t *testing.T) {
	tr := New(Config{Model: "/nao/existe/modelo.bin"})
	_, err := tr.Transcribe(context.Background(), "x.wav", igdecoder.TranscribeHint{})
	if err == nil {
		t.Error("modelo inexistente deveria falhar")
	}
}

func TestNewDefaults(t *testing.T) {
	tr := New(Config{Model: "m.bin"})
	if tr.cfg.Binary != "whisper-cli" {
		t.Errorf("binary default = %q", tr.cfg.Binary)
	}
	if tr.cfg.Language != "auto" {
		t.Errorf("language default = %q", tr.cfg.Language)
	}
}

func TestSatisfiesTranscriberInterface(t *testing.T) {
	var _ igdecoder.Transcriber = New(Config{Model: "m.bin"})
}
