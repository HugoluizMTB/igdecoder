package igdecoder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	igBase    = "https://www.instagram.com"
	igAPIBase = "https://i.instagram.com"
)

func (c *Client) apiGet(ctx context.Context, op, rawURL string, out any) error {
	if err := c.session.Validate(); err != nil {
		return err
	}
	if err := c.pace(ctx); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("igdecoder: %s: montar request: %w", op, err)
	}
	c.sign(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("igdecoder: %s: %w", op, err)
	}
	defer drain(resp.Body)

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 24<<20))
	if resp.StatusCode != http.StatusOK {
		return c.httpError(op, rawURL, resp, body)
	}

	if looksLikeLoginWall(body) {
		return &HTTPError{Op: op, Status: 200, URL: safeURL(rawURL),
			Body: truncate(body, 300), sentinel: ErrLoginRequired}
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("igdecoder: %s: json: %w (%s)", op, err, truncate(body, 200))
		}
	}
	return nil
}

func (c *Client) apiPostForm(ctx context.Context, op, rawURL string, form url.Values, out any) error {
	if err := c.session.Validate(); err != nil {
		return err
	}
	if err := c.pace(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("igdecoder: %s: montar request: %w", op, err)
	}
	c.sign(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("igdecoder: %s: %w", op, err)
	}
	defer drain(resp.Body)

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 24<<20))
	if resp.StatusCode != http.StatusOK {
		return c.httpError(op, rawURL, resp, body)
	}
	if looksLikeLoginWall(body) {
		return &HTTPError{Op: op, Status: 200, URL: safeURL(rawURL),
			Body: truncate(body, 300), sentinel: ErrLoginRequired}
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("igdecoder: %s: json: %w (%s)", op, err, truncate(body, 200))
		}
	}
	return nil
}

func (c *Client) sign(req *http.Request) {
	h := req.Header
	h.Set("User-Agent", c.session.userAgent())
	h.Set("Accept", "*/*")
	h.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")
	h.Set("X-IG-App-ID", c.session.appID())
	h.Set("X-Requested-With", "XMLHttpRequest")
	h.Set("X-ASBD-ID", "129477")
	h.Set("Referer", igBase+"/")
	h.Set("Origin", igBase)
	h.Set("Sec-Fetch-Site", "same-origin")
	h.Set("Sec-Fetch-Mode", "cors")
	h.Set("Sec-Fetch-Dest", "empty")
	if c.session.CSRFToken != "" {
		h.Set("X-CSRFToken", c.session.CSRFToken)
	}
	h.Set("Cookie", c.session.cookieHeader())
}

func (c *Client) httpError(op, rawURL string, resp *http.Response, body []byte) error {
	text := string(body)
	he := &HTTPError{
		Op:       op,
		Status:   resp.StatusCode,
		URL:      safeURL(rawURL),
		Body:     truncate(body, 400),
		sentinel: classifyStatus(resp.StatusCode, text),
	}
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(ra)); err == nil {
			he.RetryAfter = n
		}
	}
	c.log.Debug("igdecoder http erro",
		"op", op, "status", resp.StatusCode, "url", he.URL)
	return he
}

func buildURL(base, path string, q url.Values) string {
	u := base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func looksLikeLoginWall(body []byte) bool {

	s := body
	if len(s) > 4096 {
		s = s[:4096]
	}
	low := strings.ToLower(string(s))
	return strings.Contains(low, `"login_required"`) ||
		strings.Contains(low, "accounts/login/") ||
		strings.Contains(low, `"require_login"`)
}

func truncate(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func safeURL(raw string) string {
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		return raw[:i]
	}
	return raw
}
