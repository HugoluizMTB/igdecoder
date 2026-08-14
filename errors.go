package igdecoder

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrLoginRequired = errors.New("igdecoder: login exigido (sessão inválida/expirada)")

	ErrRateLimited = errors.New("igdecoder: limitado pelo instagram (rate limit)")

	ErrChallenge = errors.New("igdecoder: checkpoint/challenge na conta")

	ErrNotFound = errors.New("igdecoder: não encontrado")

	ErrPrivate = errors.New("igdecoder: perfil privado sem acesso")

	ErrNoMedia = errors.New("igdecoder: mídia sem arquivo para baixar")

	ErrUnexpectedShape = errors.New("igdecoder: formato de resposta inesperado")
)

type HTTPError struct {
	Op         string
	Status     int
	URL        string
	Body       string
	RetryAfter int
	sentinel   error
}

func (e *HTTPError) Error() string {
	msg := fmt.Sprintf("igdecoder: %s: http %d", e.Op, e.Status)
	if e.sentinel != nil {
		msg += " (" + e.sentinel.Error() + ")"
	}
	if e.Body != "" {
		msg += ": " + e.Body
	}
	return msg
}

func (e *HTTPError) Unwrap() error { return e.sentinel }

func classifyStatus(status int, body string) error {
	switch status {
	case 401:
		return ErrLoginRequired
	case 403:

		if containsAny(body, "login_required", "checkpoint") {
			if containsAny(body, "checkpoint", "challenge") {
				return ErrChallenge
			}
			return ErrLoginRequired
		}
		return ErrLoginRequired
	case 404:
		return ErrNotFound
	case 429:
		return ErrRateLimited
	}
	if status >= 500 {
		return ErrRateLimited
	}
	if containsAny(body, "checkpoint_required", "challenge_required") {
		return ErrChallenge
	}
	return nil
}

func containsAny(s string, subs ...string) bool {
	low := strings.ToLower(s)
	for _, sub := range subs {
		if sub != "" && strings.Contains(low, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
