package igdecoder

import (
	"context"
	"errors"
	"math/big"
	"net/url"
	"regexp"
	"strings"
)

var ErrBadPermalink = errors.New("igdecoder: nao consegui extrair o shortcode")

const shortcodeAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

var (
	permalinkRe = regexp.MustCompile(`instagram\.com/(?:[^/]+/)?(?:reel|reels|p|tv)/([A-Za-z0-9_-]+)`)
	shortcodeRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

func ParseShortcode(input string) (string, error) {
	s := strings.TrimSpace(input)
	if m := permalinkRe.FindStringSubmatch(s); len(m) == 2 {
		return m[1], nil
	}
	if s != "" && !strings.Contains(s, "/") && !strings.Contains(s, ".") {
		if shortcodeRe.MatchString(s) {
			return s, nil
		}
	}
	return "", ErrBadPermalink
}

func ShortcodeToMediaID(shortcode string) (string, error) {
	if shortcode == "" {
		return "", ErrBadPermalink
	}
	id := new(big.Int)
	base := big.NewInt(64)
	for _, r := range shortcode {
		idx := strings.IndexRune(shortcodeAlphabet, r)
		if idx < 0 {
			return "", ErrBadPermalink
		}
		id.Mul(id, base)
		id.Add(id, big.NewInt(int64(idx)))
	}
	return id.String(), nil
}

func (c *Client) Media(ctx context.Context, permalinkOrShortcode string) (Media, error) {
	shortcode, err := ParseShortcode(permalinkOrShortcode)
	if err != nil {
		return Media{}, err
	}
	mediaID, err := ShortcodeToMediaID(shortcode)
	if err != nil {
		return Media{}, err
	}

	var resp any
	if err := c.apiGet(ctx, "media_info",
		buildURL(igBase, "/api/v1/media/"+mediaID+"/info/", url.Values{}), &resp); err != nil {
		return Media{}, err
	}

	items := normalizeAll(resp, "", Profile{})
	if len(items) == 0 {
		return Media{}, &HTTPError{Op: "media_info", Status: 200,
			Body: "resposta sem midia", sentinel: ErrUnexpectedShape}
	}
	m := items[0]
	if m.Shortcode == "" {
		m.Shortcode = shortcode
	}
	if m.Permalink == "" {
		m.Permalink = permalink(m.Kind, m.Shortcode, m.Owner, m.ID)
	}
	return m, nil
}
