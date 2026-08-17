package igdecoder

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

func TestAnalyzeAudioReal(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("sem ffmpeg")
	}
	cli := &Client{ffmpegBin: "ffmpeg"}
	casos := []struct {
		path    string
		classe  AudioClass
		temFala bool
	}{
		{"/tmp/fala.wav", AudioSpeech, true},
		{"/tmp/misto.wav", AudioMixed, true},
		{"/tmp/musica.wav", AudioMusic, false},
	}
	for _, c := range casos {
		if _, err := os.Stat(c.path); err != nil {
			t.Skipf("falta %s", c.path)
		}
		p, err := cli.AnalyzeAudio(context.Background(), c.path)
		if err != nil {
			t.Fatalf("%s: %v", c.path, err)
		}
		t.Logf("%-17s entropy=%.4f pausas=%.1f -> %s (conf %.2f) fala=%v",
			c.path, p.Entropy, p.PausesPerMinute, p.Class, p.Confidence, p.HasSpeech())
		if p.Class != c.classe {
			t.Errorf("%s: classe %s, esperava %s", c.path, p.Class, c.classe)
		}
		if p.HasSpeech() != c.temFala {
			t.Errorf("%s: HasSpeech=%v, esperava %v", c.path, p.HasSpeech(), c.temFala)
		}
	}
}

func TestClassifyAudioLimiares(t *testing.T) {
	casos := []struct {
		entropy float64
		pausas  float64
		classe  AudioClass
	}{
		{0.20, 10, AudioSpeech},
		{0.35, 0, AudioSpeech},
		{0.50, 0, AudioMixed},
		{0.50, 10, AudioSpeech},
		{0.70, 0, AudioMusic},
		{1.50, 0, AudioMusic},
		{0, 0, AudioUnknown},
	}
	for _, c := range casos {
		p := AudioProfile{Entropy: c.entropy, PausesPerMinute: c.pausas}
		classifyAudio(&p)
		if p.Class != c.classe {
			t.Errorf("entropy=%.2f pausas=%.0f -> %s, esperava %s", c.entropy, c.pausas, p.Class, c.classe)
		}
		if p.Confidence < 0 || p.Confidence > 1 {
			t.Errorf("confianca fora de 0..1: %.2f", p.Confidence)
		}
	}
}

func TestLyricRepetition(t *testing.T) {
	sem := Transcript{Segments: []Segment{{Text: "um"}, {Text: "dois"}, {Text: "tres"}}}
	if r := LyricRepetition(sem); r != 0 {
		t.Errorf("sem repeticao deveria dar 0, deu %.2f", r)
	}
	com := Transcript{Segments: []Segment{{Text: "refrao"}, {Text: "verso"}, {Text: "REFRAO "}, {Text: "refrao"}}}
	if r := LyricRepetition(com); r < 0.4 {
		t.Errorf("repeticao alta esperada, deu %.2f", r)
	}
	if r := LyricRepetition(Transcript{Segments: []Segment{{Text: "a"}}}); r != 0 {
		t.Error("menos de 3 segmentos deveria dar 0")
	}
}
