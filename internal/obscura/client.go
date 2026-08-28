package obscura

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lsprdev/Navego/internal/browser"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	DefaultMaxChars   = 4_000
	DefaultMaxLinks   = 100
	DefaultMaxPayload = 16 << 20
)

var requiredTools = map[string][]string{
	"browser_navigate":   {"url"},
	"browser_snapshot":   {},
	"browser_markdown":   {},
	"browser_links":      {},
	"browser_screenshot": {},
	"browser_pdf":        {},
}

type Snapshot struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

type Link struct {
	Text string `json:"text"`
	Href string `json:"href"`
}

type Status struct {
	Connected     bool     `json:"connected"`
	ServerName    string   `json:"server_name,omitempty"`
	ServerVersion string   `json:"server_version,omitempty"`
	AllowedTools  []string `json:"allowed_tools,omitempty"`
}

type session interface {
	CallTool(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error)
	ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error)
	Ping(context.Context, *mcp.PingParams) error
	InitializeResult() *mcp.InitializeResult
	Close() error
}

type connector func(context.Context) (session, error)

type Client struct {
	mu sync.Mutex

	connect    connector
	session    session
	urlPolicy  *browser.PublicURLPolicy
	maxPayload int
}

func New(endpoint string, httpClient *http.Client, maxPayload int) *Client {
	endpoint = strings.TrimSpace(endpoint)
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 65 * time.Second}
	}
	if maxPayload <= 0 {
		maxPayload = DefaultMaxPayload
	}
	return newClient(func(ctx context.Context) (session, error) {
		client := mcp.NewClient(&mcp.Implementation{Name: "navego-obscura", Version: "0.1.0"}, nil)
		return client.Connect(ctx, &mcp.StreamableClientTransport{
			Endpoint:             endpoint,
			HTTPClient:           httpClient,
			DisableStandaloneSSE: true,
			MaxRetries:           1,
		}, nil)
	}, maxPayload)
}

func newClient(connect connector, maxPayload int) *Client {
	return &Client{
		connect:    connect,
		urlPolicy:  browser.NewPublicURLPolicy(),
		maxPayload: maxPayload,
	}
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	current, err := c.ensureConnectedLocked(ctx)
	if err != nil {
		return Status{}, err
	}
	if err := current.Ping(ctx, nil); err != nil {
		c.closeLocked()
		return Status{}, fmt.Errorf("ping Obscura MCP: %w", err)
	}
	info := current.InitializeResult()
	status := Status{Connected: true, AllowedTools: allowedToolNames()}
	if info != nil && info.ServerInfo != nil {
		status.ServerName = info.ServerInfo.Name
		status.ServerVersion = info.ServerInfo.Version
	}
	return status, nil
}

func (c *Client) Open(ctx context.Context, rawURL, waitUntil string, maxChars int) (Snapshot, error) {
	policyContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	u, err := c.urlPolicy.Validate(policyContext, rawURL)
	cancel()
	if err != nil {
		return Snapshot{}, err
	}
	if waitUntil == "" {
		waitUntil = "load"
	}
	if waitUntil != "load" && waitUntil != "domcontentloaded" && waitUntil != "networkidle0" {
		return Snapshot{}, fmt.Errorf("unsupported wait condition %q", waitUntil)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.callLocked(ctx, "browser_navigate", map[string]any{
		"url":       u.String(),
		"waitUntil": waitUntil,
	}); err != nil {
		return Snapshot{}, err
	}
	return c.snapshotLocked(ctx, maxChars)
}

func (c *Client) Snapshot(ctx context.Context, maxChars int) (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotLocked(ctx, maxChars)
}

func (c *Client) Markdown(ctx context.Context, maxChars int) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result, err := c.callLocked(ctx, "browser_markdown", map[string]any{"max_chars": boundedChars(maxChars)})
	if err != nil {
		return "", err
	}
	return resultText(result, c.maxPayload)
}

func (c *Client) Links(ctx context.Context, internalOnly bool, limit int) ([]Link, error) {
	if limit <= 0 {
		limit = DefaultMaxLinks
	}
	if limit > 200 {
		return nil, errors.New("link limit must not exceed 200")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	result, err := c.callLocked(ctx, "browser_links", map[string]any{
		"internal_only": internalOnly,
		"limit":         limit,
	})
	if err != nil {
		return nil, err
	}
	raw, err := resultText(result, c.maxPayload)
	if err != nil {
		return nil, err
	}
	links := make([]Link, 0)
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 1_024), c.maxPayload)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var link Link
		if err := json.Unmarshal([]byte(line), &link); err != nil {
			return nil, fmt.Errorf("decode Obscura link: %w", err)
		}
		links = append(links, link)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Obscura links: %w", err)
	}
	return links, nil
}

func (c *Client) Screenshot(ctx context.Context, width, height int) ([]byte, string, error) {
	arguments := map[string]any{}
	if width != 0 || height != 0 {
		if width < 1 || width > 4096 || height < 1 || height > 4096 {
			return nil, "", errors.New("screenshot width and height must both be between 1 and 4096")
		}
		arguments["width"] = width
		arguments["height"] = height
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	result, err := c.callLocked(ctx, "browser_screenshot", arguments)
	if err != nil {
		return nil, "", err
	}
	for _, content := range result.Content {
		image, ok := content.(*mcp.ImageContent)
		if !ok {
			continue
		}
		if image.MIMEType != "image/png" {
			return nil, "", fmt.Errorf("unexpected Obscura screenshot type %q", image.MIMEType)
		}
		if len(image.Data) > c.maxPayload {
			return nil, "", errors.New("Obscura screenshot exceeded the payload limit")
		}
		return append([]byte(nil), image.Data...), image.MIMEType, nil
	}
	return nil, "", errors.New("Obscura screenshot did not contain image data")
}

func (c *Client) PDF(ctx context.Context) ([]byte, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result, err := c.callLocked(ctx, "browser_pdf", map[string]any{"print_background": true})
	if err != nil {
		return nil, "", err
	}
	for _, content := range result.Content {
		resource, ok := content.(*mcp.EmbeddedResource)
		if !ok || resource.Resource == nil {
			continue
		}
		if resource.Resource.MIMEType != "application/pdf" {
			return nil, "", fmt.Errorf("unexpected Obscura PDF type %q", resource.Resource.MIMEType)
		}
		if len(resource.Resource.Blob) > c.maxPayload {
			return nil, "", errors.New("Obscura PDF exceeded the payload limit")
		}
		return append([]byte(nil), resource.Resource.Blob...), resource.Resource.MIMEType, nil
	}
	return nil, "", errors.New("Obscura PDF did not contain an embedded resource")
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *Client) snapshotLocked(ctx context.Context, maxChars int) (Snapshot, error) {
	result, err := c.callLocked(ctx, "browser_snapshot", map[string]any{"max_chars": boundedChars(maxChars)})
	if err != nil {
		return Snapshot{}, err
	}
	raw, err := resultText(result, c.maxPayload)
	if err != nil {
		return Snapshot{}, err
	}
	return parseSnapshot(raw), nil
}

func (c *Client) callLocked(ctx context.Context, name string, arguments map[string]any) (*mcp.CallToolResult, error) {
	if _, allowed := requiredTools[name]; !allowed {
		return nil, fmt.Errorf("Obscura tool %q is not in the adapter allowlist", name)
	}
	current, err := c.ensureConnectedLocked(ctx)
	if err != nil {
		return nil, err
	}
	result, err := current.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		c.closeLocked()
		return nil, fmt.Errorf("call Obscura %s: %w", name, err)
	}
	if result.IsError {
		if toolErr := result.GetError(); toolErr != nil {
			return nil, fmt.Errorf("Obscura %s: %w", name, toolErr)
		}
		return nil, fmt.Errorf("Obscura %s returned an error", name)
	}
	return result, nil
}

func (c *Client) ensureConnectedLocked(ctx context.Context) (session, error) {
	if c.session != nil {
		return c.session, nil
	}
	connected, err := c.connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect to Obscura MCP: %w", err)
	}
	if err := validateContract(ctx, connected); err != nil {
		connected.Close()
		return nil, err
	}
	c.session = connected
	return connected, nil
}

func (c *Client) closeLocked() error {
	if c.session == nil {
		return nil
	}
	err := c.session.Close()
	c.session = nil
	return err
}

func validateContract(ctx context.Context, connected session) error {
	info := connected.InitializeResult()
	if info == nil || info.ServerInfo == nil || info.ServerInfo.Name != "obscura-mcp" {
		return errors.New("unexpected MCP server: expected obscura-mcp")
	}
	listed, err := connected.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("list Obscura tools: %w", err)
	}
	tools := make(map[string]*mcp.Tool, len(listed.Tools))
	for _, tool := range listed.Tools {
		tools[tool.Name] = tool
	}
	for name, requiredProperties := range requiredTools {
		tool := tools[name]
		if tool == nil {
			return fmt.Errorf("Obscura contract is missing %s", name)
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			return fmt.Errorf("Obscura tool %s has an invalid input schema", name)
		}
		properties, _ := schema["properties"].(map[string]any)
		for _, property := range requiredProperties {
			if _, ok := properties[property]; !ok {
				return fmt.Errorf("Obscura tool %s is missing input property %s", name, property)
			}
		}
	}
	return nil
}

func resultText(result *mcp.CallToolResult, maxBytes int) (string, error) {
	var out strings.Builder
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok {
			continue
		}
		if out.Len()+len(text.Text) > maxBytes {
			return "", errors.New("Obscura text exceeded the payload limit")
		}
		out.WriteString(text.Text)
	}
	if out.Len() == 0 {
		return "", errors.New("Obscura response did not contain text")
	}
	return out.String(), nil
}

func parseSnapshot(raw string) Snapshot {
	result := Snapshot{Text: strings.TrimSpace(raw)}
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "URL: "):
			result.URL = strings.TrimSpace(strings.TrimPrefix(line, "URL: "))
		case strings.HasPrefix(line, "Title: "):
			result.Title = strings.TrimSpace(strings.TrimPrefix(line, "Title: "))
		}
	}
	return result
}

func boundedChars(value int) int {
	if value <= 0 {
		return DefaultMaxChars
	}
	if value > 12_000 {
		return 12_000
	}
	return value
}

func allowedToolNames() []string {
	names := make([]string, 0, len(requiredTools))
	for name := range requiredTools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
