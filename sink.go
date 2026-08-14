package igdecoder

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type FuncSink func(ctx context.Context, p Payload) error

func (f FuncSink) Deliver(ctx context.Context, p Payload) error { return f(ctx, p) }

type WebhookSink struct {
	URL        string
	Secret     string
	SignHeader string
	Headers    map[string]string
	HTTPClient *http.Client
	Timeout    time.Duration
}

func (w WebhookSink) Deliver(ctx context.Context, p Payload) error {
	if w.URL == "" {
		return fmt.Errorf("igdecoder: WebhookSink.URL vazio")
	}
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("igdecoder: webhook marshal: %w", err)
	}

	if w.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, w.Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("igdecoder: webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", p.IdempotencyKey)
	req.Header.Set("X-Igdecoder-Document", p.DocumentID)
	for k, v := range w.Headers {
		req.Header.Set(k, v)
	}
	if w.Secret != "" {
		mac := hmac.New(sha256.New, []byte(w.Secret))
		mac.Write(body)
		header := w.SignHeader
		if header == "" {
			header = "X-Signature-256"
		}
		req.Header.Set(header, "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	client := w.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("igdecoder: webhook enviar: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &HTTPError{Op: "webhook", Status: resp.StatusCode, URL: w.URL,
			Body: string(snippet)}
	}
	return nil
}

type MultiSink []Sink

func (ms MultiSink) Deliver(ctx context.Context, p Payload) error {
	var firstErr error
	for _, s := range ms {
		if err := s.Deliver(ctx, p); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
