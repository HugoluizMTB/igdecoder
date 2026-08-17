package igdecoder

import (
	"errors"
	"testing"
)

func TestClassifyStatus(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   error
	}{
		{401, "", ErrLoginRequired},
		{403, "login_required", ErrLoginRequired},
		{403, "checkpoint_required", ErrChallenge},
		{404, "", ErrNotFound},
		{429, "", ErrRateLimited},
		{500, "", ErrRateLimited},
		{503, "", ErrRateLimited},
		{200, "", nil},
	}
	for _, c := range cases {
		got := classifyStatus(c.status, c.body)
		if c.want == nil {
			if got != nil {
				t.Errorf("status %d/%q: got %v, queria nil", c.status, c.body, got)
			}
			continue
		}
		if !errors.Is(got, c.want) {
			t.Errorf("status %d/%q: got %v, queria %v", c.status, c.body, got, c.want)
		}
	}
}

func TestHTTPErrorUnwrap(t *testing.T) {
	he := &HTTPError{Op: "x", Status: 429, sentinel: ErrRateLimited}
	if !errors.Is(he, ErrRateLimited) {
		t.Error("HTTPError deveria embrulhar ErrRateLimited")
	}
}

func TestSafeURLStripsQuery(t *testing.T) {
	if got := safeURL("https://x/y?a=1&b=2"); got != "https://x/y" {
		t.Errorf("got %q", got)
	}
	if got := safeURL("https://x/y"); got != "https://x/y" {
		t.Errorf("got %q", got)
	}
}

func TestLooksLikeLoginWall(t *testing.T) {
	if !looksLikeLoginWall([]byte(`{"login_required":1}`)) {
		t.Error("deveria detectar login_required")
	}
	if !looksLikeLoginWall([]byte(`<a href="/accounts/login/">`)) {
		t.Error("deveria detectar accounts/login")
	}
	if looksLikeLoginWall([]byte(`{"data":{"user":{}}}`)) {
		t.Error("falso positivo em resposta válida")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate([]byte("abcdef"), 3); got != "abc…" {
		t.Errorf("got %q", got)
	}
	if got := truncate([]byte("ab"), 3); got != "ab" {
		t.Errorf("got %q", got)
	}
}

func TestBuildURL(t *testing.T) {
	if got := buildURL("https://x", "/p", nil); got != "https://x/p" {
		t.Errorf("got %q", got)
	}
	got := buildURL("https://x", "/p", map[string][]string{"a": {"1"}})
	if got != "https://x/p?a=1" {
		t.Errorf("got %q", got)
	}
}

func TestSessionDefaults(t *testing.T) {
	if (Session{}).appID() != DefaultAppID {
		t.Error("appID default")
	}
	if (Session{}).userAgent() != DefaultUserAgent {
		t.Error("userAgent default")
	}
	if (Session{AppID: "x"}).appID() != "x" {
		t.Error("appID override")
	}
}

func TestCookieHeader(t *testing.T) {
	full := Session{SessionID: "a", UserID: "b", CSRFToken: "c"}
	if got := full.cookieHeader(); got != "sessionid=a; ds_user_id=b; csrftoken=c" {
		t.Errorf("got %q", got)
	}
	if got := (Session{SessionID: "a"}).cookieHeader(); got != "sessionid=a" {
		t.Errorf("parcial: got %q", got)
	}
}

func TestThrottleNotMistakenForLogin(t *testing.T) {
	body := `{"message":"Aguarde alguns minutos antes de tentar novamente.","require_login":true,"status":"fail"}`
	got := classifyStatus(401, body)
	if !errors.Is(got, ErrRateLimited) {
		t.Errorf("throttle 401 deveria ser ErrRateLimited, veio %v", got)
	}
	if errors.Is(got, ErrLoginRequired) {
		t.Error("throttle nao deveria pedir novo login")
	}
	real := classifyStatus(401, `{"message":"login_required"}`)
	if !errors.Is(real, ErrLoginRequired) {
		t.Errorf("401 real deveria ser ErrLoginRequired, veio %v", real)
	}
}

func TestChallengeTemPrioridadeSobreThrottle(t *testing.T) {
	body := `{"message":"checkpoint_required. Please try again later.","status":"fail"}`
	got := classifyStatus(403, body)
	if !errors.Is(got, ErrChallenge) {
		t.Errorf("checkpoint com texto de throttle deveria ser ErrChallenge, veio %v", got)
	}
	if errors.Is(got, ErrRateLimited) {
		t.Error("checkpoint nao pode virar rate limit: causaria retry infinito")
	}
}
