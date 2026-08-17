package mihomo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

type CommandValidator struct {
	Binary  string
	DataDir string
	Timeout time.Duration
}

func (v CommandValidator) Validate(ctx context.Context, candidatePath string) error {
	validationContext, cancel := context.WithTimeout(ctx, v.Timeout)
	defer cancel()
	command := exec.CommandContext(validationContext, v.Binary, "-t", "-f", candidatePath, "-d", v.DataDir)
	var output bytes.Buffer
	command.Stdout = &limitedWriter{writer: &output, remaining: 4 << 10}
	command.Stderr = &limitedWriter{writer: &output, remaining: 4 << 10}
	if err := command.Run(); err != nil {
		if errors.Is(validationContext.Err(), context.DeadlineExceeded) {
			return errors.New("mihomo validation timed out")
		}
		return fmt.Errorf("mihomo rejected the candidate: %w", err)
	}
	return nil
}

type Client struct {
	baseURL *url.URL
	secret  string
	client  *http.Client
}

func NewClient(rawURL, secret string, timeout time.Duration) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(rawURL, "/"))
	if err != nil || parsed.Host == "" {
		return nil, errors.New("invalid Mihomo Controller URL")
	}
	return &Client{
		baseURL: parsed,
		secret:  secret,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) Reload(ctx context.Context, configPath string) error {
	payload, err := json.Marshal(map[string]string{"path": configPath})
	if err != nil {
		return errors.New("encode reload request")
	}
	endpoint := c.resolve("/configs?force=true")
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(payload))
	if err != nil {
		return errors.New("create reload request")
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.do(req, nil); err != nil {
		return fmt.Errorf("reload Mihomo: %w", err)
	}
	return nil
}

func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolve("/version"), nil)
	if err != nil {
		return errors.New("create health request")
	}
	var response struct {
		Version string `json:"version"`
	}
	if err := c.do(req, &response); err != nil {
		return fmt.Errorf("Mihomo health check: %w", err)
	}
	if strings.TrimSpace(response.Version) == "" {
		return errors.New("Mihomo health check returned no version")
	}
	return nil
}

func (c *Client) Version(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolve("/version"), nil)
	if err != nil {
		return "", errors.New("create version request")
	}
	var response struct {
		Version string `json:"version"`
	}
	if err := c.do(req, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.Version) == "" {
		return "", errors.New("Mihomo returned no version")
	}
	return response.Version, nil
}

func (c *Client) do(req *http.Request, output any) error {
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	response, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return errors.New("request timed out")
		}
		return errors.New("request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return fmt.Errorf("unexpected HTTP status %d", response.StatusCode)
	}
	if output == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(output); err != nil {
		return errors.New("invalid JSON response")
	}
	return nil
}

func (c *Client) resolve(path string) string {
	reference, _ := url.Parse(path)
	return c.baseURL.ResolveReference(reference).String()
}

type limitedWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *limitedWriter) Write(data []byte) (int, error) {
	originalLength := len(data)
	if w.remaining <= 0 {
		return originalLength, nil
	}
	if int64(len(data)) > w.remaining {
		data = data[:w.remaining]
	}
	written, err := w.writer.Write(data)
	w.remaining -= int64(written)
	if err != nil {
		return written, err
	}
	return originalLength, nil
}
