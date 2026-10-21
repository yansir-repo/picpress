package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// refreshMargin: credentials with less than this left are refreshed first.
// For lifetimes shorter than twice the margin, half the lifetime is used
// instead, so short-lived credentials are not refetched on every request.
const refreshMargin = 5 * time.Minute

// CredentialSource produces credentials. It is the only thing that differs
// between a token config (F1/F2) and a hand-entered S3 endpoint (F3).
type CredentialSource interface {
	Fetch(ctx context.Context) (Credentials, error)
}

// StaticSource returns fixed credentials (F3, development).
type StaticSource struct{ Creds Credentials }

func (s StaticSource) Fetch(context.Context) (Credentials, error) {
	if err := s.Creds.validate(); err != nil {
		return Credentials{}, &FatalError{Reason: ReasonAuth, Message: "自定义 S3 地址配置有误：" + err.Error()}
	}
	return s.Creds, nil
}

// DeviceInfo is sent with every credentials request.
type DeviceInfo struct {
	DeviceID   string `json:"deviceId"`
	AppVersion string `json:"appVersion"`
	OS         string `json:"os"`
}

// TokenSource exchanges a config token for credentials (docs §5).
type TokenSource struct {
	URL    string
	Token  string
	Device DeviceInfo
	HTTP   *http.Client
}

func (s TokenSource) Fetch(ctx context.Context) (Credentials, error) {
	if _, err := CheckURL(s.URL); err != nil {
		return Credentials{}, &FatalError{Reason: ReasonAuth, Message: "配置里的地址不可用：" + err.Error()}
	}
	body, _ := json.Marshal(s.Device)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(body))
	if err != nil {
		return Credentials{}, err
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set("Content-Type", "application/json")
	client := s.HTTP
	if client == nil {
		client = noRedirectClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return Credentials{}, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusUnauthorized:
		return Credentials{}, &FatalError{Reason: ReasonAuth, Message: msgRevoked}
	case resp.StatusCode == http.StatusForbidden:
		return Credentials{}, &FatalError{Reason: ReasonAuth, Message: msgDevice}
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		e := &S3Error{Status: resp.StatusCode, Code: "CredentialsUnavailable"}
		if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
			e.RetryAfter = time.Duration(secs) * time.Second
		}
		return Credentials{}, e
	default:
		return Credentials{}, &FatalError{Reason: ReasonAuth, Message: fmt.Sprintf("换取上传凭证失败（%d），请联系给你配置的人。", resp.StatusCode)}
	}

	var c Credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return Credentials{}, &S3Error{Status: 502, Code: "BadCredentials", Message: "invalid JSON"}
	}
	if err := c.validate(); err != nil {
		return Credentials{}, &FatalError{Reason: ReasonAuth, Message: "服务端返回的凭证有误：" + err.Error()}
	}
	return c, nil
}

// Session caches credentials and refreshes them before they expire or when
// the server rejects them. Safe for concurrent use.
type Session struct {
	Source CredentialSource
	Now    func() time.Time

	mu      sync.Mutex
	cur     *Credentials
	fetched time.Time
	gen     int
}

// Get returns usable credentials and their generation.
func (s *Session) Get(ctx context.Context) (Credentials, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur != nil && s.fresh(s.now()) {
		return *s.cur, s.gen, nil
	}
	c, err := s.Source.Fetch(ctx)
	if err != nil {
		return Credentials{}, s.gen, err
	}
	s.cur, s.fetched = &c, s.now()
	s.gen++
	return c, s.gen, nil
}

func (s *Session) fresh(now time.Time) bool {
	if s.cur.ExpiresAt.IsZero() {
		return true
	}
	margin := min(refreshMargin, s.cur.ExpiresAt.Sub(s.fetched)/2)
	return now.Add(margin).Before(s.cur.ExpiresAt)
}

// Invalidate drops the credentials of generation gen, so the next Get fetches
// new ones. Concurrent workers that saw the same rejection refresh only once.
func (s *Session) Invalidate(gen int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen == gen {
		s.cur = nil
	}
}

// Endpoint is the S3 endpoint of the cached credentials ("" before the first
// successful fetch). Used by the reachability probe.
func (s *Session) Endpoint() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil {
		return ""
	}
	return s.cur.Endpoint
}

func (s *Session) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
