package igdecoder

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	EntropyMusicThreshold  = 0.70
	EntropySpeechThreshold = 0.35
	speechPauseRate        = 4.0
	greyZoneMaxConfidence  = 0.6
)

type AudioClass string

const (
	AudioSpeech  AudioClass = "speech"
	AudioMixed   AudioClass = "mixed"
	AudioMusic   AudioClass = "music"
	AudioUnknown AudioClass = "unknown"
)

type AudioProfile struct {
	Entropy         float64       `json:"entropy"`
	Flatness        float64       `json:"flatness"`
	Centroid        float64       `json:"centroid"`
	Rolloff         float64       `json:"rolloff"`
	PausesPerMinute float64       `json:"pauses_per_minute"`
	Duration        time.Duration `json:"duration"`
	Class           AudioClass    `json:"class"`
	Confidence      float64       `json:"confidence"`
}

func (p AudioProfile) HasSpeech() bool {
	return p.Class == AudioSpeech || p.Class == AudioMixed
}

func (p AudioProfile) LikelyMusic() bool {
	return p.Class == AudioMusic || p.Class == AudioMixed
}

var (
	spectralRe = regexp.MustCompile(`lavfi\.aspectralstats\.\d+\.(\w+)=([0-9.eE+-]+)`)
	durationRe = regexp.MustCompile(`Duration: (\d+):(\d+):(\d+\.\d+)`)
)

func (c *Client) AnalyzeAudio(ctx context.Context, wavPath string) (AudioProfile, error) {
	args := []string{
		"-nostdin", "-hide_banner", "-nostats",
		"-i", wavPath,
		"-af", "aspectralstats,ametadata=mode=print,silencedetect=noise=-32dB:d=0.20",
		"-f", "null", "-",
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.ffmpegBin, args...)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return AudioProfile{}, ctx.Err()
		}
		if isExecNotFound(err) {
			return AudioProfile{}, fmt.Errorf(
				"igdecoder: analyze audio: ffmpeg nao encontrado (%q): instale ou use WithFFmpegPath: %w",
				c.ffmpegBin, err)
		}
		return AudioProfile{}, fmt.Errorf("igdecoder: analyze audio: %w: %s",
			err, truncate(stderr.Bytes(), 300))
	}

	p := parseAudioStats(stderr.String())
	classifyAudio(&p)
	return p, nil
}

func parseAudioStats(out string) AudioProfile {
	sums := map[string]float64{}
	counts := map[string]int{}
	for _, m := range spectralRe.FindAllStringSubmatch(out, -1) {
		v, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			continue
		}
		sums[m[1]] += v
		counts[m[1]]++
	}
	mean := func(k string) float64 {
		if counts[k] == 0 {
			return 0
		}
		return sums[k] / float64(counts[k])
	}

	p := AudioProfile{
		Entropy:  mean("entropy"),
		Flatness: mean("flatness"),
		Centroid: mean("centroid"),
		Rolloff:  mean("rolloff"),
		Duration: parseFFmpegDuration(out),
	}

	pauses := strings.Count(out, "silence_start")
	if mins := p.Duration.Minutes(); mins > 0 {
		p.PausesPerMinute = float64(pauses) / mins
	}
	return p
}

func parseFFmpegDuration(out string) time.Duration {
	m := durationRe.FindStringSubmatch(out)
	if len(m) != 4 {
		return 0
	}
	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	s, _ := strconv.ParseFloat(m[3], 64)
	return time.Duration(float64(h)*3600+float64(min)*60+s) * time.Second
}

func classifyAudio(p *AudioProfile) {
	switch {
	case p.Entropy <= 0:
		p.Class = AudioUnknown
		p.Confidence = 0
	case p.Entropy >= EntropyMusicThreshold:
		p.Class = AudioMusic
		p.Confidence = saturate((p.Entropy - EntropyMusicThreshold) / EntropyMusicThreshold)
	case p.Entropy <= EntropySpeechThreshold:
		p.Class = AudioSpeech
		p.Confidence = saturate((EntropySpeechThreshold - p.Entropy) / EntropySpeechThreshold)
	default:
		p.Class = AudioMixed
		span := EntropyMusicThreshold - EntropySpeechThreshold
		meio := EntropySpeechThreshold + span/2
		perto := 1 - saturate(abs(p.Entropy-meio)/(span/2))
		if p.PausesPerMinute >= speechPauseRate {
			p.Class = AudioSpeech
		}
		p.Confidence = perto * greyZoneMaxConfidence
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func saturate(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}

var wsRe = regexp.MustCompile(`\s+`)

func LyricRepetition(t Transcript) float64 {
	if len(t.Segments) < 3 {
		return 0
	}
	seen := map[string]int{}
	total := 0
	for _, s := range t.Segments {
		line := wsRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s.Text)), " ")
		if line == "" {
			continue
		}
		seen[line]++
		total++
	}
	if total == 0 {
		return 0
	}
	return 1 - float64(len(seen))/float64(total)
}
