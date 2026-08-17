package subscription

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/3xiaoxiao3/mihomo-stack/internal/config"
	"gopkg.in/yaml.v3"
)

type Builder struct {
	subscription config.SubscriptionConfig
	converter    config.ConverterConfig
	client       *http.Client
	readSecret   func(string) (string, error)
}

func NewBuilder(subscription config.SubscriptionConfig, converter config.ConverterConfig) *Builder {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	client := &http.Client{
		Timeout:   subscription.RequestTimeout.Duration,
		Transport: transport,
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if len(via) > 0 && !sameHost(via[0].URL, req.URL) {
			for name := range req.Header {
				if !strings.EqualFold(name, "User-Agent") && !strings.EqualFold(name, "Accept") {
					req.Header.Del(name)
				}
			}
		}
		return nil
	}
	return &Builder{
		subscription: subscription,
		converter:    converter,
		client:       client,
		readSecret:   config.ReadSecret,
	}
}

func (b *Builder) Build(ctx context.Context) ([]byte, error) {
	type resolvedSource struct {
		name    string
		url     string
		headers map[string]string
	}
	var sources []resolvedSource
	for _, source := range b.subscription.Sources {
		if !source.IsEnabled() {
			continue
		}
		rawURL, err := b.readSecret(source.URLFile)
		if err != nil {
			return nil, fmt.Errorf("subscription %q: %w", source.Name, err)
		}
		if err := validateSourceURL(rawURL); err != nil {
			return nil, fmt.Errorf("subscription %q: %w", source.Name, err)
		}
		sources = append(sources, resolvedSource{name: source.Name, url: rawURL, headers: source.Headers})
	}
	if len(sources) == 0 {
		return nil, errors.New("no enabled subscriptions")
	}

	if b.converter.Enabled {
		urls := make([]string, 0, len(sources))
		for _, source := range sources {
			if len(source.headers) != 0 {
				return nil, fmt.Errorf("subscription %q: custom headers are not supported in converter mode", source.name)
			}
			urls = append(urls, source.url)
		}
		body, err := b.fetchConverted(ctx, urls)
		if err != nil {
			return nil, err
		}
		return normalizeDocument(body, b.subscription.MaxBytes)
	}

	documents := make([]map[string]any, len(sources))
	fetchErrors := make([]error, len(sources))
	var fetchGroup sync.WaitGroup
	for index := range sources {
		fetchGroup.Add(1)
		go func() {
			defer fetchGroup.Done()
			source := sources[index]
			body, err := b.fetch(ctx, source.url, source.headers)
			if err == nil {
				documents[index], err = decodeDocument(body)
			}
			if err != nil {
				fetchErrors[index] = fmt.Errorf("subscription %q: %w", source.name, err)
			}
		}()
	}
	fetchGroup.Wait()
	for _, err := range fetchErrors {
		if err != nil {
			return nil, err
		}
	}

	merged, err := mergeDocuments(documents)
	if err != nil {
		return nil, err
	}
	result, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("encode merged subscription: %w", err)
	}
	return result, nil
}

func (b *Builder) fetch(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.New("create subscription request")
	}
	req.Header.Set("Accept", "application/yaml, text/yaml, text/plain, */*")
	req.Header.Set("User-Agent", b.subscription.UserAgent)
	for name, value := range headers {
		if strings.EqualFold(name, "Host") || strings.EqualFold(name, "Content-Length") {
			return nil, fmt.Errorf("header %q is not allowed", name)
		}
		req.Header.Set(name, value)
	}
	return b.do(req)
}

func (b *Builder) fetchConverted(ctx context.Context, sourceURLs []string) ([]byte, error) {
	endpoint, err := url.Parse(strings.TrimRight(b.converter.APIURL, "/") + "/sub")
	if err != nil {
		return nil, errors.New("invalid converter endpoint")
	}
	query := endpoint.Query()
	query.Set("target", "clash")
	query.Set("url", strings.Join(sourceURLs, "|"))
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, errors.New("create converter request")
	}
	req.Header.Set("Accept", "application/yaml, text/yaml, text/plain")
	req.Header.Set("User-Agent", b.subscription.UserAgent)
	body, err := b.do(req)
	if err != nil {
		return nil, fmt.Errorf("converter request failed: %w", err)
	}
	return body, nil
}

func (b *Builder) do(req *http.Request) ([]byte, error) {
	response, err := b.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("request timed out")
		}
		return nil, errors.New("request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected HTTP status %d", response.StatusCode)
	}
	limited := io.LimitReader(response.Body, b.subscription.MaxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, errors.New("read response")
	}
	if int64(len(body)) > b.subscription.MaxBytes {
		return nil, errors.New("response exceeds configured size limit")
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("response is empty")
	}
	if looksLikeHTML(response.Header.Get("Content-Type"), body) {
		return nil, errors.New("response appears to be HTML, not a subscription")
	}
	return body, nil
}

func normalizeDocument(body []byte, maxBytes int64) ([]byte, error) {
	if int64(len(body)) > maxBytes {
		return nil, errors.New("response exceeds configured size limit")
	}
	document, err := decodeDocument(body)
	if err != nil {
		return nil, err
	}
	return yaml.Marshal(document)
}

func decodeDocument(body []byte) (map[string]any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, errors.New("response is not valid YAML")
	}
	if len(document) == 0 {
		return nil, errors.New("YAML document is empty")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("multiple YAML documents are not supported")
	}
	return document, nil
}

func mergeDocuments(documents []map[string]any) (map[string]any, error) {
	if len(documents) == 0 {
		return nil, errors.New("no subscription documents to merge")
	}
	result := cloneMap(documents[0])
	for _, document := range documents[1:] {
		if err := mergeNamedList(result, document, "proxies", false); err != nil {
			return nil, err
		}
		if err := mergeProviderMap(result, document); err != nil {
			return nil, err
		}
		if err := mergeNamedList(result, document, "proxy-groups", true); err != nil {
			return nil, err
		}
		mergeRules(result, document)
	}
	if !hasNonEmpty(result, "proxies") && !hasNonEmpty(result, "proxy-providers") && !hasNonEmpty(result, "rules") {
		return nil, errors.New("merged subscription has no proxies, providers, or rules")
	}
	return result, nil
}

func mergeNamedList(target, source map[string]any, key string, mergeMembers bool) error {
	incoming, ok := asSlice(source[key])
	if !ok || len(incoming) == 0 {
		return nil
	}
	existing, _ := asSlice(target[key])
	index := make(map[string]int, len(existing))
	for i, item := range existing {
		name, err := itemName(item, key)
		if err != nil {
			return err
		}
		index[name] = i
	}
	for _, item := range incoming {
		name, err := itemName(item, key)
		if err != nil {
			return err
		}
		position, found := index[name]
		if !found {
			existing = append(existing, item)
			index[name] = len(existing) - 1
			continue
		}
		if reflect.DeepEqual(existing[position], item) {
			continue
		}
		if mergeMembers {
			merged, err := mergeGroup(existing[position], item, name)
			if err != nil {
				return err
			}
			existing[position] = merged
			continue
		}
		return fmt.Errorf("conflicting %s entry %q", key, name)
	}
	target[key] = existing
	return nil
}

func mergeGroup(leftValue, rightValue any, name string) (map[string]any, error) {
	left, leftOK := leftValue.(map[string]any)
	right, rightOK := rightValue.(map[string]any)
	if !leftOK || !rightOK {
		return nil, fmt.Errorf("proxy-groups entry %q must be a map", name)
	}
	leftWithoutMembers := cloneMap(left)
	rightWithoutMembers := cloneMap(right)
	delete(leftWithoutMembers, "proxies")
	delete(rightWithoutMembers, "proxies")
	if !reflect.DeepEqual(leftWithoutMembers, rightWithoutMembers) {
		return nil, fmt.Errorf("conflicting proxy-groups entry %q", name)
	}
	leftMembers, _ := asSlice(left["proxies"])
	rightMembers, _ := asSlice(right["proxies"])
	seen := make(map[string]struct{}, len(leftMembers))
	for _, member := range leftMembers {
		seen[fmt.Sprint(member)] = struct{}{}
	}
	for _, member := range rightMembers {
		key := fmt.Sprint(member)
		if _, exists := seen[key]; !exists {
			leftMembers = append(leftMembers, member)
			seen[key] = struct{}{}
		}
	}
	merged := cloneMap(left)
	merged["proxies"] = leftMembers
	return merged, nil
}

func mergeProviderMap(target, source map[string]any) error {
	incoming, ok := source["proxy-providers"].(map[string]any)
	if !ok || len(incoming) == 0 {
		return nil
	}
	existing, _ := target["proxy-providers"].(map[string]any)
	if existing == nil {
		existing = make(map[string]any)
	}
	keys := make([]string, 0, len(incoming))
	for name := range incoming {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		if _, found := existing[name]; found {
			return fmt.Errorf("duplicate proxy-provider %q", name)
		}
		existing[name] = incoming[name]
	}
	target["proxy-providers"] = existing
	return nil
}

func mergeRules(target, source map[string]any) {
	incoming, ok := asSlice(source["rules"])
	if !ok {
		return
	}
	existing, _ := asSlice(target["rules"])
	seen := make(map[string]struct{}, len(existing))
	for _, rule := range existing {
		seen[fmt.Sprint(rule)] = struct{}{}
	}
	for _, rule := range incoming {
		key := fmt.Sprint(rule)
		if _, found := seen[key]; !found {
			existing = append(existing, rule)
			seen[key] = struct{}{}
		}
	}
	target["rules"] = existing
}

func itemName(value any, kind string) (string, error) {
	item, ok := value.(map[string]any)
	if !ok {
		return "", fmt.Errorf("%s entry must be a map", kind)
	}
	name, ok := item["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("%s entry requires a name", kind)
	}
	return name, nil
}

func asSlice(value any) ([]any, bool) {
	items, ok := value.([]any)
	return items, ok
}

func hasNonEmpty(document map[string]any, key string) bool {
	switch value := document[key].(type) {
	case []any:
		return len(value) > 0
	case map[string]any:
		return len(value) > 0
	default:
		return false
	}
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func validateSourceURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("secret does not contain an absolute HTTP(S) URL")
	}
	return nil
}

func sameHost(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}

func looksLikeHTML(contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return true
	}
	prefix := strings.ToLower(string(bytes.TrimSpace(body)))
	return strings.HasPrefix(prefix, "<!doctype html") || strings.HasPrefix(prefix, "<html")
}

// SetHTTPClient is intended for tests that need a controlled transport.
func (b *Builder) SetHTTPClient(client *http.Client) {
	b.client = client
}

// SetSecretReader is intended for tests that avoid writing secret files.
func (b *Builder) SetSecretReader(reader func(string) (string, error)) {
	b.readSecret = reader
}
