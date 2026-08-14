package igdecoder

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type routeFunc func(r *http.Request) (int, string)

func newFakeClient(t *testing.T, route routeFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, body := route(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	base := srv.Listener.Addr().String()
	rt := rewriteTransport{host: base}
	cli, err := New(
		Session{SessionID: "test", UserID: "1", CSRFToken: "c"},
		WithHTTPClient(&http.Client{Transport: rt, Timeout: 5 * time.Second}),
		WithPacing(0, 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	return cli
}

type rewriteTransport struct{ host string }

func (rt rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme = "http"
	r.URL.Host = rt.host
	return http.DefaultTransport.RoundTrip(r)
}

const profileJSON = `{"data":{"user":{
	"id":"456","username":"fulano","full_name":"Fulano",
	"is_verified":true,"is_private":false,
	"edge_followed_by":{"count":9000}
}}}`

func reelJSON(pk, code string, takenAt int64) string {
	return fmt.Sprintf(`{"media":{
		"pk":%q,"code":%q,"media_type":2,"taken_at":%d,
		"video_versions":[{"url":"https://cdn/%s.mp4","width":720,"height":1280}],
		"video_duration":10.0,"caption":{"text":"legenda"},
		"user":{"pk":"456","username":"fulano"},
		"play_count":100,"like_count":10
	}}`, pk, code, takenAt, pk)
}

func TestProfileParses(t *testing.T) {
	cli := newFakeClient(t, func(r *http.Request) (int, string) {
		if !strings.Contains(r.URL.Path, "web_profile_info") {
			return 404, `{}`
		}
		return 200, profileJSON
	})
	p, err := cli.Profile(context.Background(), "@Fulano")
	if err != nil {
		t.Fatal(err)
	}
	if p.Username != "fulano" || p.UserID != "456" {
		t.Errorf("perfil = %+v", p)
	}
	if !p.Verified || p.Followers != 9000 {
		t.Errorf("campos = %+v", p)
	}
}

func TestProfileUnexpectedShape(t *testing.T) {
	cli := newFakeClient(t, func(r *http.Request) (int, string) {
		return 200, `{"data":{}}`
	})
	_, err := cli.Profile(context.Background(), "x")
	if !errors.Is(err, ErrUnexpectedShape) {
		t.Errorf("erro = %v, queria ErrUnexpectedShape", err)
	}
}

func TestProfileLoginRequired(t *testing.T) {
	cli := newFakeClient(t, func(r *http.Request) (int, string) {
		return 401, `{"message":"login_required"}`
	})
	_, err := cli.Profile(context.Background(), "x")
	if !errors.Is(err, ErrLoginRequired) {
		t.Errorf("erro = %v, queria ErrLoginRequired", err)
	}
}

func TestCaptureReelsAndStories(t *testing.T) {
	now := time.Now().Unix()
	cli := newFakeClient(t, func(r *http.Request) (int, string) {
		switch {
		case strings.Contains(r.URL.Path, "web_profile_info"):
			return 200, profileJSON
		case strings.Contains(r.URL.Path, "clips/user"):
			return 200, `{"items":[` + reelJSON("1", "AAA", now) + `],"paging_info":{"more_available":false}}`
		case strings.Contains(r.URL.Path, "reels_media"):
			return 200, fmt.Sprintf(`{"reels":{"456":{"items":[{
				"pk":"9","media_type":2,"taken_at":%d,"expiring_at":%d,
				"video_versions":[{"url":"https://cdn/s.mp4","width":720,"height":1280}],
				"user":{"pk":"456","username":"fulano"}
			}]}}}`, now, now+86400)
		}
		return 404, `{}`
	})

	medias, err := cli.Capture(context.Background(), "fulano", Filter{
		Kinds: []Kind{Reel, Story}, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(medias) != 2 {
		t.Fatalf("esperava 2 mídias, veio %d", len(medias))
	}
	kinds := map[Kind]bool{}
	for _, m := range medias {
		kinds[m.Kind] = true
		if m.Owner.Username != "fulano" {
			t.Errorf("owner = %q", m.Owner.Username)
		}
	}
	if !kinds[Reel] || !kinds[Story] {
		t.Errorf("faltou tipo: %v", kinds)
	}
}

func TestCaptureRespectsSince(t *testing.T) {
	old := time.Now().Add(-30 * 24 * time.Hour).Unix()
	cli := newFakeClient(t, func(r *http.Request) (int, string) {
		if strings.Contains(r.URL.Path, "web_profile_info") {
			return 200, profileJSON
		}
		if strings.Contains(r.URL.Path, "clips/user") {
			return 200, `{"items":[` + reelJSON("1", "OLD", old) + `],"paging_info":{"more_available":false}}`
		}
		return 404, `{}`
	})
	medias, err := cli.Capture(context.Background(), "fulano", Filter{
		Kinds: []Kind{Reel}, Since: 7 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(medias) != 0 {
		t.Errorf("mídia antiga deveria ser filtrada, veio %d", len(medias))
	}
}

func TestCapturePaginatesUntilLimit(t *testing.T) {
	now := time.Now().Unix()
	calls := 0
	cli := newFakeClient(t, func(r *http.Request) (int, string) {
		if strings.Contains(r.URL.Path, "web_profile_info") {
			return 200, profileJSON
		}
		if strings.Contains(r.URL.Path, "clips/user") {
			calls++
			if calls == 1 {
				return 200, `{"items":[` + reelJSON("1", "A", now) + `],"paging_info":{"more_available":true,"max_id":"CURSOR"}}`
			}
			return 200, `{"items":[` + reelJSON("2", "B", now) + `],"paging_info":{"more_available":false}}`
		}
		return 404, `{}`
	})
	medias, err := cli.Capture(context.Background(), "fulano", Filter{Kinds: []Kind{Reel}, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Errorf("deveria paginar, chamadas = %d", calls)
	}
	if len(medias) != 2 {
		t.Errorf("esperava 2 mídias, veio %d", len(medias))
	}
}

func TestCaptureFatalStopsEverything(t *testing.T) {
	cli := newFakeClient(t, func(r *http.Request) (int, string) {
		if strings.Contains(r.URL.Path, "web_profile_info") {
			return 200, profileJSON
		}
		return 429, `{"message":"rate limited"}`
	})
	_, err := cli.Capture(context.Background(), "fulano", Filter{Kinds: []Kind{Reel, Story}})
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("erro = %v, queria ErrRateLimited", err)
	}
}

func TestCaptureContextCancel(t *testing.T) {
	cli := newFakeClient(t, func(r *http.Request) (int, string) {
		return 200, profileJSON
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cli.Capture(ctx, "fulano", Filter{}); err == nil {
		t.Error("contexto cancelado deveria falhar")
	}
}

func TestFilterDefaults(t *testing.T) {
	var f Filter
	if got := f.kinds(); len(got) != 2 || got[0] != Reel || got[1] != Story {
		t.Errorf("kinds default = %v", got)
	}
	if f.limit() != DefaultLimit || f.maxPages() != DefaultMaxPages {
		t.Error("limites default")
	}
	if !f.cutoff().IsZero() {
		t.Error("cutoff sem Since deveria ser zero")
	}
	if (Filter{Since: time.Hour}).cutoff().IsZero() {
		t.Error("cutoff com Since não deveria ser zero")
	}
}

func TestDedupeSortOrdersNewestFirst(t *testing.T) {
	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	got := dedupeSort([]Media{
		{ID: "a", TakenAt: older},
		{ID: "b", TakenAt: newer},
		{ID: "a", TakenAt: older},
	})
	if len(got) != 2 {
		t.Fatalf("dedupe falhou: %d", len(got))
	}
	if got[0].ID != "b" {
		t.Errorf("ordem errada: %s primeiro", got[0].ID)
	}
}

func TestNormUsername(t *testing.T) {
	if normUsername("  @Fulano ") != "fulano" {
		t.Error("normUsername")
	}
}

func TestIsFatal(t *testing.T) {
	if !isFatal(ErrLoginRequired) || !isFatal(ErrChallenge) || !isFatal(ErrRateLimited) {
		t.Error("erros fatais")
	}
	if isFatal(ErrNotFound) {
		t.Error("ErrNotFound não é fatal")
	}
}

func TestCapSlice(t *testing.T) {
	in := []Media{{ID: "1"}, {ID: "2"}, {ID: "3"}}
	if len(capSlice(in, 2)) != 2 {
		t.Error("capSlice deveria truncar")
	}
	if len(capSlice(in, 0)) != 3 {
		t.Error("limite 0 não trunca")
	}
}
