package mihomo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientAuthenticatesReloadAndHealth(t *testing.T) {
	var reloadSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer controller-secret" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/configs":
			if request.Method != http.MethodPut || request.URL.Query().Get("force") != "true" {
				t.Fatalf("unexpected reload request: %s %s", request.Method, request.URL.String())
			}
			reloadSeen = true
			response.WriteHeader(http.StatusNoContent)
		case "/version":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"version":"v1.2.3"}`))
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "controller-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Reload(context.Background(), "/data/config.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := client.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reloadSeen {
		t.Fatal("reload endpoint was not called")
	}
}

func TestClientDoesNotIncludeResponseBodyInError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusInternalServerError)
		_, _ = response.Write([]byte("secret response body"))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	err = client.Health(context.Background())
	if err == nil {
		t.Fatal("expected health error")
	}
	if got := err.Error(); got == "" || contains(got, "secret response body") {
		t.Fatalf("unexpected error = %q", got)
	}
}

func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
