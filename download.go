package igdecoder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (c *Client) Download(ctx context.Context, m Media, dir string) (string, error) {
	src := m.MediaURL()
	if src == "" {
		return "", ErrNoMedia
	}
	if dir == "" {
		dir = c.tempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("igdecoder: download: mkdir: %w", err)
	}

	if err := c.pace(ctx); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return "", fmt.Errorf("igdecoder: download: %w", err)
	}
	req.Header.Set("User-Agent", c.session.userAgent())
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", igBase+"/")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("igdecoder: download: %w", err)
	}
	defer drain(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", &HTTPError{Op: "download", Status: resp.StatusCode, URL: safeURL(src),
			sentinel: classifyStatus(resp.StatusCode, "")}
	}

	name := m.ID
	if name == "" {
		name = randomName(src)
	}
	dest := filepath.Join(dir, name+mediaExt(m, resp.Header.Get("Content-Type")))
	f, err := os.Create(dest)
	if err != nil {
		return "", fmt.Errorf("igdecoder: download: criar arquivo: %w", err)
	}

	limited := io.LimitReader(resp.Body, c.maxBytes+1)
	n, copyErr := io.Copy(f, limited)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(dest)
		return "", fmt.Errorf("igdecoder: download: copiar: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(dest)
		return "", fmt.Errorf("igdecoder: download: fechar: %w", closeErr)
	}
	if n > c.maxBytes {
		_ = os.Remove(dest)
		return "", fmt.Errorf("igdecoder: download: excede o limite de %d bytes", c.maxBytes)
	}
	c.log.Debug("igdecoder: baixado", "media", m.ID, "bytes", n, "path", dest)
	return dest, nil
}

func (c *Client) tempDir() string {
	if c.workDir != "" {
		return c.workDir
	}
	return os.TempDir()
}

func mediaExt(m Media, contentType string) string {
	if m.IsVideo() {
		return ".mp4"
	}
	if strings.Contains(contentType, "video") {
		return ".mp4"
	}
	if strings.Contains(contentType, "png") {
		return ".png"
	}
	return ".jpg"
}

func randomName(src string) string {
	sum := sha256.Sum256([]byte(src))
	return hex.EncodeToString(sum[:])[:24]
}
