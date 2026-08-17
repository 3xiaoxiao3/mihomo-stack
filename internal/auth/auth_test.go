package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionAuthenticationAndSameOrigin(t *testing.T) {
	manager := New("a-long-random-token", time.Hour)
	loginRequest := httptest.NewRequest(http.MethodPost, "http://guardian.test/api/v1/auth/login", nil)
	loginResponse := httptest.NewRecorder()
	manager.SetSession(loginResponse, loginRequest)
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie = %#v", cookies)
	}

	called := false
	handler := manager.Require(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}), func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
	})
	request := httptest.NewRequest(http.MethodPost, "http://guardian.test/api/v1/updates", nil)
	request.AddCookie(cookies[0])
	request.Header.Set("Origin", "http://guardian.test")
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if !called {
		t.Fatal("authenticated same-origin request was rejected")
	}
}

func TestSessionRejectsCrossOriginMutation(t *testing.T) {
	manager := New("a-long-random-token", time.Hour)
	loginResponse := httptest.NewRecorder()
	manager.SetSession(loginResponse, httptest.NewRequest(http.MethodPost, "http://guardian.test/login", nil))
	cookie := loginResponse.Result().Cookies()[0]

	status := 0
	handler := manager.Require(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("cross-origin request reached handler")
	}), func(response http.ResponseWriter, _ *http.Request) {
		status = http.StatusUnauthorized
		response.WriteHeader(status)
	})
	request := httptest.NewRequest(http.MethodPost, "http://guardian.test/api/v1/updates", nil)
	request.AddCookie(cookie)
	request.Header.Set("Origin", "http://evil.test")
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d", status)
	}
}

func TestBearerAuthenticationDoesNotRequireOrigin(t *testing.T) {
	manager := New("token", time.Hour)
	called := false
	handler := manager.Require(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}), func(http.ResponseWriter, *http.Request) {
		t.Fatal("bearer request was rejected")
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/updates", nil)
	request.Header.Set("Authorization", "Bearer token")
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if !called {
		t.Fatal("handler was not called")
	}
}
