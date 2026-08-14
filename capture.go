package igdecoder

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"
)

type Filter struct {
	Kinds []Kind

	Since time.Duration

	Limit int

	MaxPages int
}

const (
	DefaultLimit    = 30
	DefaultMaxPages = 10
	pageSize        = 12
)

func (f Filter) kinds() []Kind {
	if len(f.Kinds) > 0 {
		return f.Kinds
	}
	return []Kind{Reel, Story}
}

func (f Filter) limit() int {
	if f.Limit > 0 {
		return f.Limit
	}
	return DefaultLimit
}

func (f Filter) maxPages() int {
	if f.MaxPages > 0 {
		return f.MaxPages
	}
	return DefaultMaxPages
}

func (f Filter) cutoff() time.Time {
	if f.Since <= 0 {
		return time.Time{}
	}
	return time.Now().Add(-f.Since)
}

func (c *Client) Capture(ctx context.Context, username string, f Filter) ([]Media, error) {
	username = normUsername(username)
	prof, err := c.Profile(ctx, username)
	if err != nil {
		return nil, err
	}
	if prof.Private {

		c.log.Debug("igdecoder: perfil privado", "username", username)
	}

	seen := map[string]bool{}
	var all []Media
	cutoff := f.cutoff()

	for _, kind := range f.kinds() {
		var (
			items []Media
			kerr  error
		)
		switch kind {
		case Reel:
			items, kerr = c.reels(ctx, prof, f.limit(), f.maxPages(), cutoff)
		case Story:
			items, kerr = c.stories(ctx, prof)
		case Video, Image:
			items, kerr = c.posts(ctx, prof, f.limit(), f.maxPages(), cutoff)
		case Highlight:
			items, kerr = c.highlights(ctx, prof, f.limit())
		default:
			continue
		}
		if kerr != nil {

			if isFatal(kerr) {
				return dedupeSort(all), kerr
			}
			c.log.Warn("igdecoder: coleta parcial", "kind", kind, "username", username, "err", kerr)
			continue
		}
		for _, m := range items {
			if m.ID == "" || seen[m.ID] {
				continue
			}
			if !cutoff.IsZero() && !m.TakenAt.IsZero() && m.TakenAt.Before(cutoff) {
				continue
			}
			if len(f.Kinds) > 0 && !kindWanted(f.Kinds, m.Kind) {

				continue
			}
			seen[m.ID] = true
			all = append(all, m)
		}
	}
	return dedupeSort(all), nil
}

func (c *Client) Profile(ctx context.Context, username string) (Profile, error) {
	q := url.Values{"username": {normUsername(username)}}
	var resp any
	err := c.apiGet(ctx, "profile",
		buildURL(igBase, "/api/v1/users/web_profile_info/", q), &resp)
	if err != nil {
		return Profile{}, err
	}
	u, ok := asMap(dig(resp, "data", "user"))
	if !ok || u == nil {
		return Profile{}, &HTTPError{Op: "profile", Status: 200,
			Body: "sem data.user", sentinel: ErrUnexpectedShape}
	}
	p := Profile{
		Username:  strings.ToLower(digStr(u, "username")),
		UserID:    orStr(digStr(u, "id"), digStr(u, "pk")),
		FullName:  digStr(u, "full_name"),
		Verified:  toBool(u["is_verified"]),
		Private:   toBool(u["is_private"]),
		PicURL:    orStr(digStr(u, "profile_pic_url_hd"), digStr(u, "profile_pic_url")),
		Followers: digInt(u, "edge_followed_by", "count"),
	}
	if p.Username == "" || p.UserID == "" {
		return p, &HTTPError{Op: "profile", Status: 200,
			Body: "perfil sem id/username", sentinel: ErrUnexpectedShape}
	}
	return p, nil
}

func (c *Client) Reels(ctx context.Context, username string, limit int) ([]Media, error) {
	prof, err := c.Profile(ctx, username)
	if err != nil {
		return nil, err
	}
	return c.reels(ctx, prof, orInt(limit, DefaultLimit), DefaultMaxPages, time.Time{})
}

func (c *Client) Stories(ctx context.Context, username string) ([]Media, error) {
	prof, err := c.Profile(ctx, username)
	if err != nil {
		return nil, err
	}
	return c.stories(ctx, prof)
}

func (c *Client) Posts(ctx context.Context, username string, limit int) ([]Media, error) {
	prof, err := c.Profile(ctx, username)
	if err != nil {
		return nil, err
	}
	return c.posts(ctx, prof, orInt(limit, DefaultLimit), DefaultMaxPages, time.Time{})
}

func (c *Client) feed(ctx context.Context, op string, prof Profile, limit, maxPages int, cutoff time.Time, keep func(Kind) bool) ([]Media, error) {
	var out []Media
	cursor := ""
	for page := 0; page < maxPages && len(out) < limit; page++ {
		q := url.Values{"count": {itoa(pageSize)}}
		if cursor != "" {
			q.Set("max_id", cursor)
		}
		var resp any
		if err := c.apiGet(ctx, op,
			buildURL(igBase, "/api/v1/feed/user/"+prof.UserID+"/", q), &resp); err != nil {
			if page == 0 {
				return nil, err
			}
			break
		}
		batch := normalizeAll(resp, "", prof)
		for _, m := range batch {
			if keep(m.Kind) {
				out = append(out, m)
			}
		}
		if reachedCutoff(batch, cutoff) {
			break
		}
		cursor = nextCursor(resp)
		if cursor == "" {
			break
		}
	}
	return capSlice(out, limit), nil
}

func (c *Client) reels(ctx context.Context, prof Profile, limit, maxPages int, cutoff time.Time) ([]Media, error) {
	return c.feed(ctx, "reels", prof, limit, maxPages, cutoff, func(k Kind) bool {
		return k == Reel
	})
}

func (c *Client) posts(ctx context.Context, prof Profile, limit, maxPages int, cutoff time.Time) ([]Media, error) {
	return c.feed(ctx, "posts", prof, limit, maxPages, cutoff, func(k Kind) bool {
		return k == Video || k == Image
	})
}

func (c *Client) stories(ctx context.Context, prof Profile) ([]Media, error) {
	q := url.Values{"reel_ids": {prof.UserID}}
	var resp any
	if err := c.apiGet(ctx, "stories",
		buildURL(igBase, "/api/v1/feed/reels_media/", q), &resp); err != nil {
		return nil, err
	}
	return normalizeAll(resp, Story, prof), nil
}

func (c *Client) highlights(ctx context.Context, prof Profile, limit int) ([]Media, error) {
	var tray any
	if err := c.apiGet(ctx, "highlights_tray",
		buildURL(igBase, "/api/v1/highlights/"+prof.UserID+"/highlights_tray/", nil), &tray); err != nil {
		return nil, err
	}

	var ids []string
	walkMaps(tray, func(m jmap) bool {
		if id := digStr(m, "id"); strings.HasPrefix(id, "highlight:") {
			ids = append(ids, id)
		}
		return true
	})
	var out []Media
	for _, id := range ids {
		if len(out) >= limit {
			break
		}
		q := url.Values{"reel_ids": {id}}
		var resp any
		if err := c.apiGet(ctx, "highlight_reel",
			buildURL(igBase, "/api/v1/feed/reels_media/", q), &resp); err != nil {
			if isFatal(err) {
				return out, err
			}
			continue
		}
		out = append(out, normalizeAll(resp, Highlight, prof)...)
	}
	return capSlice(out, limit), nil
}

func normalizeAll(resp any, kindHint Kind, owner Profile) []Media {
	nodes := collectMediaNodes(resp)
	var out []Media
	for _, n := range nodes {
		for _, m := range normNode(n, kindHint, owner, "http") {
			if m.Owner.Username == "" {
				m.Owner = owner
			}
			out = append(out, m)
		}
	}
	return out
}

func reachedCutoff(batch []Media, cutoff time.Time) bool {
	if cutoff.IsZero() {
		return false
	}
	for _, m := range batch {
		if !m.TakenAt.IsZero() && m.TakenAt.Before(cutoff) {
			return true
		}
	}
	return false
}

func dedupeSort(items []Media) []Media {
	seen := map[string]bool{}
	out := items[:0]
	for _, m := range items {
		if m.ID == "" || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].TakenAt.After(out[j].TakenAt)
	})
	return out
}

func capSlice(items []Media, limit int) []Media {
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func isFatal(err error) bool {
	return errors.Is(err, ErrLoginRequired) ||
		errors.Is(err, ErrChallenge) ||
		errors.Is(err, ErrRateLimited)
}

func kindWanted(kinds []Kind, k Kind) bool {
	return slices.Contains(kinds, k)
}

func normUsername(u string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(u), "@"))
}

func orInt(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}
