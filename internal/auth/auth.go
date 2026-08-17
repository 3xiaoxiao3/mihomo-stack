package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const sessionCookie = "guardian_session"

type contextKey string

const cookieAuthKey contextKey = "cookie-auth"

type Manager struct {
	token []byte
	ttl   time.Duration
	now   func() time.Time
}

func New(token string, ttl time.Duration) *Manager {
	return &Manager{token: []byte(token), ttl: ttl, now: time.Now}
}

func (m *Manager) Enabled() bool {
	return len(m.token) > 0
}

func (m *Manager) ValidateToken(candidate string) bool {
	if !m.Enabled() {
		return true
	}
	value := []byte(candidate)
	return len(value) == len(m.token) && subtle.ConstantTimeCompare(value, m.token) == 1
}

func (m *Manager) SetSession(response http.ResponseWriter, request *http.Request) {
	expires := m.now().Add(m.ttl).UTC()
	payload := strconv.FormatInt(expires.Unix(), 10)
	mac := hmac.New(sha256.New, m.sessionKey())
	_, _ = mac.Write([]byte(payload))
	value := payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	http.SetCookie(response, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(m.ttl.Seconds()),
		HttpOnly: true,
		Secure:   request.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
}

func (m *Manager) ClearSession(response http.ResponseWriter, request *http.Request) {
	http.SetCookie(response, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   request.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
}

func (m *Manager) Authenticate(request *http.Request) (context.Context, bool) {
	if !m.Enabled() {
		return request.Context(), true
	}
	if header := request.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		if m.ValidateToken(strings.TrimPrefix(header, "Bearer ")) {
			return context.WithValue(request.Context(), cookieAuthKey, false), true
		}
	}
	cookie, err := request.Cookie(sessionCookie)
	if err != nil || !m.validSession(cookie.Value) {
		return request.Context(), false
	}
	return context.WithValue(request.Context(), cookieAuthKey, true), true
}

func (m *Manager) Require(next http.Handler, unauthorized func(http.ResponseWriter, *http.Request)) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		ctx, ok := m.Authenticate(request)
		if !ok {
			unauthorized(response, request)
			return
		}
		request = request.WithContext(ctx)
		if cookieAuthenticated(ctx) && isMutation(request.Method) && !sameOrigin(request) {
			unauthorized(response, request)
			return
		}
		next.ServeHTTP(response, request)
	})
}

func (m *Manager) validSession(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return false
	}
	expiresUnix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || !m.now().Before(time.Unix(expiresUnix, 0)) {
		return false
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, m.sessionKey())
	_, _ = mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	return len(provided) == len(expected) && subtle.ConstantTimeCompare(provided, expected) == 1
}

func (m *Manager) sessionKey() []byte {
	hash := sha256.Sum256(append([]byte("mihomo-guardian-session:"), m.token...))
	return hash[:]
}

func sameOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	expectedScheme := "http"
	if request.TLS != nil {
		expectedScheme = "https"
	}
	return parsed.Scheme == expectedScheme && strings.EqualFold(parsed.Host, request.Host)
}

func cookieAuthenticated(ctx context.Context) bool {
	value, _ := ctx.Value(cookieAuthKey).(bool)
	return value
}

func isMutation(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

var ErrInvalidToken = errors.New("invalid administrator token")
