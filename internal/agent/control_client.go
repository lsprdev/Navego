package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type HTTPControlClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPControlClient(baseURL, token string, client *http.Client) (*HTTPControlClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid control URL")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("agent token is required")
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPControlClient{baseURL: baseURL, token: strings.TrimSpace(token), client: client}, nil
}

func (c *HTTPControlClient) Commands(ctx context.Context, agentID string) ([]Browser, error) {
	query := url.Values{"agent_id": []string{agentID}}
	var result []Browser
	if err := c.do(ctx, http.MethodGet, "/api/navego/internal/commands?"+query.Encode(), nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *HTTPControlClient) Report(ctx context.Context, browserID string, report StateReport) error {
	return c.do(ctx, http.MethodPatch, "/api/navego/internal/browsers/"+url.PathEscape(browserID), report, nil)
}

func (c *HTTPControlClient) ConfirmDeletion(ctx context.Context, browserID string) error {
	return c.do(ctx, http.MethodDelete, "/api/navego/internal/browsers/"+url.PathEscape(browserID), nil, nil)
}

func (c *HTTPControlClient) do(ctx context.Context, method, path string, body, result any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode control request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create control request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	response, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("control request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("control returned %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	if result == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(result); err != nil {
		return fmt.Errorf("decode control response: %w", err)
	}
	return nil
}
