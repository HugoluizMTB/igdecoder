package igdecoder

import (
	"context"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

type Transcriber interface {
	Transcribe(ctx context.Context, audioPath string, hint TranscribeHint) (Transcript, error)
}

type TranscribeHint struct {
	Language string
	Duration time.Duration
	MediaID  string
}

type Sink interface {
	Deliver(ctx context.Context, p Payload) error
}

type Client struct {
	session Session
	http    *http.Client
	log     *slog.Logger

	minDelay, maxDelay time.Duration
	maxBytes           int64
	maxDuration        time.Duration

	ffmpegBin string
	workDir   string

	rng    *rand.Rand
	rngMu  sync.Mutex
	paceMu sync.Mutex
	lastAt time.Time
}

type Option func(*Client)

func New(s Session, opts ...Option) (*Client, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	c := &Client{
		session:     s,
		http:        defaultHTTPClient(),
		log:         slog.Default(),
		minDelay:    2 * time.Second,
		maxDelay:    6 * time.Second,
		maxBytes:    512 << 20,
		maxDuration: 30 * time.Minute,
		ffmpegBin:   "ffmpeg",
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

func WithLogger(l *slog.Logger) Option {
	return func(c *Client) {
		if l != nil {
			c.log = l
		}
	}
}

func WithPacing(min, max time.Duration) Option {
	return func(c *Client) {
		if min < 0 {
			min = 0
		}
		if max < min {
			max = min
		}
		c.minDelay, c.maxDelay = min, max
	}
}

func WithLimits(maxBytes int64, maxDuration time.Duration) Option {
	return func(c *Client) {
		if maxBytes > 0 {
			c.maxBytes = maxBytes
		}
		if maxDuration > 0 {
			c.maxDuration = maxDuration
		}
	}
}

func WithFFmpegPath(path string) Option {
	return func(c *Client) {
		if path != "" {
			c.ffmpegBin = path
		}
	}
}

func WithWorkDir(dir string) Option {
	return func(c *Client) { c.workDir = dir }
}

func (c *Client) Session() Session { return c.session }

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 90 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        20,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     60 * time.Second,
			ForceAttemptHTTP2:   true,
		},
	}
}

func (c *Client) pace(ctx context.Context) error {
	c.paceMu.Lock()
	defer c.paceMu.Unlock()

	wait := c.jitter()
	if !c.lastAt.IsZero() {
		if elapsed := time.Since(c.lastAt); elapsed < wait {
			wait -= elapsed
		} else {
			wait = 0
		}
	} else {
		wait = 0
	}
	if wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	c.lastAt = time.Now()
	return nil
}

func (c *Client) jitter() time.Duration {
	if c.maxDelay <= 0 {
		return 0
	}
	c.rngMu.Lock()
	defer c.rngMu.Unlock()
	span := c.maxDelay - c.minDelay
	if span <= 0 {
		return c.minDelay
	}

	f := betaish(c.rng)
	d := c.minDelay + time.Duration(f*float64(span))
	if c.rng.Float64() < 0.08 {
		d += time.Duration(c.rng.Float64() * float64(span))
	}
	return d
}

func betaish(r *rand.Rand) float64 {
	a := (r.Float64() + r.Float64()) / 2
	return a * a
}

func drain(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 1<<20))
	_ = body.Close()
}
