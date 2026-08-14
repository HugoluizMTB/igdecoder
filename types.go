package igdecoder

import (
	"encoding/json"
	"time"
)

type Kind string

const (
	Reel      Kind = "reel"
	Video     Kind = "video"
	Story     Kind = "story"
	Highlight Kind = "highlight"
	Image     Kind = "image"
	Unknown   Kind = "unknown"
)

func (k Kind) IsVideoKind() bool {
	switch k {
	case Reel, Video, Story, Highlight:
		return true
	default:
		return false
	}
}

type Profile struct {
	Username  string `json:"username"`
	UserID    string `json:"user_id,omitempty"`
	FullName  string `json:"full_name,omitempty"`
	Verified  bool   `json:"verified,omitempty"`
	Private   bool   `json:"private,omitempty"`
	Followers int64  `json:"followers,omitempty"`
	PicURL    string `json:"pic_url,omitempty"`
}

type Media struct {
	ID        string        `json:"id"`
	Kind      Kind          `json:"kind"`
	Owner     Profile       `json:"owner"`
	TakenAt   time.Time     `json:"taken_at"`
	ExpiresAt time.Time     `json:"expires_at,omitempty"`
	Shortcode string        `json:"shortcode,omitempty"`
	Permalink string        `json:"permalink,omitempty"`
	Caption   string        `json:"caption,omitempty"`
	Duration  time.Duration `json:"duration,omitempty"`
	Width     int           `json:"width,omitempty"`
	Height    int           `json:"height,omitempty"`
	HasAudio  bool          `json:"has_audio,omitempty"`

	Views    int64 `json:"views,omitempty"`
	Likes    int64 `json:"likes,omitempty"`
	Comments int64 `json:"comments,omitempty"`

	AudioTitle  string `json:"audio_title,omitempty"`
	AudioArtist string `json:"audio_artist,omitempty"`

	VideoURL string `json:"-"`
	ImageURL string `json:"-"`

	Raw json.RawMessage `json:"-"`
}

func (m Media) IsVideo() bool { return m.VideoURL != "" }

func (m Media) MediaURL() string {
	if m.VideoURL != "" {
		return m.VideoURL
	}
	return m.ImageURL
}

type Segment struct {
	Start time.Duration `json:"start"`
	End   time.Duration `json:"end"`
	Text  string        `json:"text"`
}

type Transcript struct {
	Text     string        `json:"text"`
	Language string        `json:"language,omitempty"`
	Segments []Segment     `json:"segments,omitempty"`
	Engine   string        `json:"engine,omitempty"`
	Model    string        `json:"model,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
	NoSpeech bool          `json:"no_speech,omitempty"`
}

type Chunk struct {
	Index  int           `json:"index"`
	Total  int           `json:"total"`
	Text   string        `json:"text"`
	Start  time.Duration `json:"start,omitempty"`
	End    time.Duration `json:"end,omitempty"`
	Tokens int           `json:"tokens"`
}

type Payload struct {
	SchemaVersion  string    `json:"schema_version"`
	DocumentID     string    `json:"document_id"`
	MediaID        string    `json:"media_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Profile        Profile   `json:"profile"`
	Kind           Kind      `json:"kind"`
	TakenAt        time.Time `json:"taken_at"`
	Permalink      string    `json:"permalink,omitempty"`
	Caption        string    `json:"caption,omitempty"`
	Language       string    `json:"language,omitempty"`
	Duration       float64   `json:"duration_s,omitempty"`
	AudioTitle     string    `json:"audio_title,omitempty"`
	Metrics        Metrics   `json:"metrics,omitempty"`
	Chunk          Chunk     `json:"chunk"`
	CapturedAt     time.Time `json:"captured_at"`
}

type Metrics struct {
	Views    int64 `json:"views,omitempty"`
	Likes    int64 `json:"likes,omitempty"`
	Comments int64 `json:"comments,omitempty"`
}
