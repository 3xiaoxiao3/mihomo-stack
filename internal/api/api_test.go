package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/3xiaoxiao3/mihomo-stack/internal/auth"
	"github.com/3xiaoxiao3/mihomo-stack/internal/state"
	"github.com/3xiaoxiao3/mihomo-stack/internal/updater"
)

type fakeUpdates struct {
	busy bool
}

func (f *fakeUpdates) Update(context.Context, string) (state.Record, error) {
	return state.Record{ID: "update", Success: true}, nil
}

func (f *fakeUpdates) Restore(context.Context, string, string) (state.Record, error) {
	return state.Record{ID: "restore", Success: true}, nil
}

func (f *fakeUpdates) ListBackups() ([]updater.Backup, error) { return []updater.Backup{}, nil }
func (f *fakeUpdates) Busy() bool                             { return f.busy }

type fakeHistory struct{}

func (fakeHistory) History() []state.Record { return []state.Record{} }

type fakeMihomo struct{}

func (fakeMihomo) Health(context.Context) error            { return nil }
func (fakeMihomo) Version(context.Context) (string, error) { return "v1.0.0", nil }

func testServer(t *testing.T) *Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(Options{Version: "test", UpdateInterval: time.Hour}, auth.New("admin-token", time.Hour), &fakeUpdates{}, fakeHistory{}, fakeMihomo{}, logger)
}

func TestProtectedEndpointRequiresAuthentication(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers are missing")
	}
}

func TestLoginAndCookieUpdate(t *testing.T) {
	server := testServer(t)
	loginRequest := httptest.NewRequest(http.MethodPost, "http://guardian.test/api/v1/auth/login", strings.NewReader(`{"token":"admin-token"}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginResponse.Code, loginResponse.Body.String())
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %#v", cookies)
	}

	updateRequest := httptest.NewRequest(http.MethodPost, "http://guardian.test/api/v1/updates", nil)
	updateRequest.AddCookie(cookies[0])
	updateRequest.Header.Set("Origin", "http://guardian.test")
	updateResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updateResponse.Code, updateResponse.Body.String())
	}
}

func TestLoginRejectsUnknownJSONField(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"token":"admin-token","extra":true}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", response.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] == nil {
		t.Fatalf("body = %#v", body)
	}
}

func TestUnknownAPIRouteReturnsJSONNotSPA(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("content type = %q", contentType)
	}
}
