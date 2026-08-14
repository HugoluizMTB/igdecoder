package igdecoder

import (
	"strings"
	"time"
)

type ChunkOptions struct {
	MaxTokens      int
	OverlapTokens  int
	IncludeCaption bool
}

const (
	DefaultMaxTokens     = 900
	DefaultOverlapTokens = 60
)

func (o ChunkOptions) maxTokens() int {
	if o.MaxTokens > 0 {
		return o.MaxTokens
	}
	return DefaultMaxTokens
}

func (o ChunkOptions) overlap() int {
	if o.OverlapTokens > 0 {
		return o.OverlapTokens
	}
	return DefaultOverlapTokens
}

type unit struct {
	text   string
	start  time.Duration
	end    time.Duration
	tokens int
}

func estimateTokens(s string) int {
	n := len([]rune(s))
	if n == 0 {
		return 0
	}
	t := n * 10 / 38
	if t < 1 {
		return 1
	}
	return t
}

func Chunks(t Transcript, m Media, opts ChunkOptions) []Payload {
	units := unitsFrom(t)
	if opts.IncludeCaption && strings.TrimSpace(m.Caption) != "" {
		cap := strings.TrimSpace(m.Caption)
		units = append([]unit{{text: cap, tokens: estimateTokens(cap)}}, units...)
	}
	if len(units) == 0 {
		return nil
	}

	chunks := groupUnits(units, opts.maxTokens(), opts.overlap())
	if len(chunks) == 0 {
		return nil
	}

	docID := m.ID
	if docID == "" {
		docID = stableID(string(m.Kind), m.Shortcode, m.Permalink)
	}
	total := len(chunks)
	now := time.Now().UTC()

	payloads := make([]Payload, 0, total)
	for idx := range chunks {
		ch := chunks[idx]
		ch.Index = idx
		ch.Total = total
		payloads = append(payloads, Payload{
			SchemaVersion:  "1",
			DocumentID:     docID,
			MediaID:        m.ID,
			IdempotencyKey: docID + ":" + itoa(idx),
			Profile:        m.Owner,
			Kind:           m.Kind,
			TakenAt:        m.TakenAt,
			Permalink:      m.Permalink,
			Caption:        m.Caption,
			Language:       t.Language,
			Duration:       m.Duration.Seconds(),
			AudioTitle:     m.AudioTitle,
			Metrics:        Metrics{Views: m.Views, Likes: m.Likes, Comments: m.Comments},
			Chunk:          ch,
			CapturedAt:     now,
		})
	}
	return payloads
}

func unitsFrom(t Transcript) []unit {
	if len(t.Segments) > 0 {
		us := make([]unit, 0, len(t.Segments))
		for _, s := range t.Segments {
			txt := strings.TrimSpace(s.Text)
			if txt == "" {
				continue
			}
			us = append(us, unit{text: txt, start: s.Start, end: s.End, tokens: estimateTokens(txt)})
		}
		return us
	}
	var us []unit
	for _, sent := range splitSentences(t.Text) {
		sent = strings.TrimSpace(sent)
		if sent == "" {
			continue
		}
		us = append(us, unit{text: sent, tokens: estimateTokens(sent)})
	}
	return us
}

func groupUnits(units []unit, max, overlap int) []Chunk {
	var chunks []Chunk
	i := 0
	for i < len(units) {
		var group []unit
		tok := 0
		for i < len(units) && (len(group) == 0 || tok+units[i].tokens <= max) {
			group = append(group, units[i])
			tok += units[i].tokens
			i++
		}
		chunks = append(chunks, buildChunk(group))
		if i < len(units) && overlap > 0 && len(group) > 1 {
			back, j := 0, len(group)-1
			for j > 0 && back < overlap {
				back += group[j].tokens
				j--
			}
			rewind := len(group) - 1 - j
			if rewind > 0 && rewind < len(group) {
				i -= rewind
			}
		}
	}
	return chunks
}

func buildChunk(group []unit) Chunk {
	var sb strings.Builder
	tok := 0
	for i, u := range group {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(u.text)
		tok += u.tokens
	}
	ch := Chunk{Text: strings.TrimSpace(sb.String()), Tokens: tok}
	if len(group) > 0 {
		ch.Start = group[0].start
		ch.End = group[len(group)-1].end
	}
	return ch
}

func splitSentences(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var out []string
	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		runes := []rune(para)
		start := 0
		for i := 0; i < len(runes); i++ {
			c := runes[i]
			if c == '.' || c == '!' || c == '?' {
				if i+1 >= len(runes) || runes[i+1] == ' ' {
					out = append(out, string(runes[start:i+1]))
					start = i + 1
				}
			}
		}
		if start < len(runes) {
			out = append(out, string(runes[start:]))
		}
	}
	return out
}
