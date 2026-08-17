package subscription

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/3xiaoxiao3/mihomo-stack/internal/config"
	"gopkg.in/yaml.v3"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestBuildMergesSubscriptions(t *testing.T) {
	cfg := config.SubscriptionConfig{
		MaxBytes:       1 << 20,
		RequestTimeout: config.Duration{Duration: time.Second},
		UserAgent:      "test",
		Sources: []config.SubscriptionSource{
			{Name: "one", URLFile: "one"},
			{Name: "two", URLFile: "two"},
		},
	}
	builder := NewBuilder(cfg, config.ConverterConfig{})
	builder.SetSecretReader(func(path string) (string, error) {
		return "https://subscriptions.test/" + path, nil
	})
	builder.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `proxies:
  - name: alpha
    type: ss
rules:
  - MATCH,alpha
`
		if strings.HasSuffix(request.URL.Path, "two") {
			body = `proxies:
  - name: beta
    type: ss
rules:
  - DOMAIN,example.com,beta
  - MATCH,alpha
`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})})

	result, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(result, &document); err != nil {
		t.Fatal(err)
	}
	if got := len(document["proxies"].([]any)); got != 2 {
		t.Fatalf("proxy count = %d", got)
	}
	if got := len(document["rules"].([]any)); got != 2 {
		t.Fatalf("rule count = %d", got)
	}
}

func TestBuildDoesNotLeakURLOnFailure(t *testing.T) {
	cfg := config.SubscriptionConfig{
		MaxBytes:       1 << 20,
		RequestTimeout: config.Duration{Duration: time.Second},
		Sources:        []config.SubscriptionSource{{Name: "private", URLFile: "secret"}},
	}
	builder := NewBuilder(cfg, config.ConverterConfig{})
	const secretURL = "https://example.test/sub?token=very-secret"
	builder.SetSecretReader(func(string) (string, error) { return secretURL, nil })
	builder.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})})

	_, err := builder.Build(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "very-secret") || strings.Contains(err.Error(), secretURL) {
		t.Fatalf("error leaked URL: %v", err)
	}
}

func TestMergeRejectsConflictingNamedProxy(t *testing.T) {
	documents := []map[string]any{
		{"proxies": []any{map[string]any{"name": "same", "type": "ss"}}},
		{"proxies": []any{map[string]any{"name": "same", "type": "vmess"}}},
	}
	_, err := mergeDocuments(documents)
	if err == nil || !strings.Contains(err.Error(), "conflicting proxies") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestDecodeRejectsHTMLAndMultipleDocuments(t *testing.T) {
	if !looksLikeHTML("text/plain", []byte("<!doctype html><title>error</title>")) {
		t.Fatal("expected HTML detection")
	}
	_, err := decodeDocument([]byte("proxies: []\n---\nrules: []\n"))
	if err == nil {
		t.Fatal("expected multiple-document error")
	}
}
