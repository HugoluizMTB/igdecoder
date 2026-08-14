package igdecoder

import (
	"errors"
	"os"
	"strings"
)

type Session struct {
	SessionID string
	UserID    string
	CSRFToken string

	UserAgent string

	AppID string
}

const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

const DefaultAppID = "936619743392459"

var ErrNoSession = errors.New("igdecoder: sessão sem sessionid")

func (s Session) Validate() error {
	if strings.TrimSpace(s.SessionID) == "" {
		return ErrNoSession
	}
	return nil
}

func (s Session) userAgent() string {
	if s.UserAgent != "" {
		return s.UserAgent
	}
	return DefaultUserAgent
}

func (s Session) appID() string {
	if s.AppID != "" {
		return s.AppID
	}
	return DefaultAppID
}

func (s Session) cookieHeader() string {
	var b strings.Builder
	write := func(name, val string) {
		if val == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		b.WriteString(name)
		b.WriteByte('=')
		b.WriteString(val)
	}
	write("sessionid", s.SessionID)
	write("ds_user_id", s.UserID)
	write("csrftoken", s.CSRFToken)
	return b.String()
}

func SessionFromEnv() (Session, error) {
	s := Session{
		SessionID: os.Getenv("IGDEC_SESSIONID"),
		UserID:    os.Getenv("IGDEC_USERID"),
		CSRFToken: os.Getenv("IGDEC_CSRF"),
		UserAgent: os.Getenv("IGDEC_USERAGENT"),
		AppID:     os.Getenv("IGDEC_APPID"),
	}
	return s, s.Validate()
}

func SessionFromCookieString(raw string) (Session, error) {
	var s Session
	fields := map[string]*string{
		"sessionid":  &s.SessionID,
		"ds_user_id": &s.UserID,
		"csrftoken":  &s.CSRFToken,
	}
	assign := func(name, val string) {
		if p, ok := fields[strings.ToLower(strings.TrimSpace(name))]; ok {
			*p = strings.TrimSpace(val)
		}
	}
	for _, line := range strings.Split(raw, "\n") {
		for _, part := range strings.Split(line, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if i := strings.IndexByte(part, '='); i >= 0 {
				assign(part[:i], part[i+1:])
			} else if i := strings.IndexByte(part, '\t'); i >= 0 {
				assign(part[:i], part[i+1:])
			}
		}
	}
	return s, s.Validate()
}
