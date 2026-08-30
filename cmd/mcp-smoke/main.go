package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	endpoint := envOr("MCP_SMOKE_ENDPOINT", "http://127.0.0.1:8001/mcp")
	targetURL := envOr("MCP_SMOKE_URL", "https://example.com")
	apiKey := strings.TrimSpace(os.Getenv("NAVEGO_API_KEY"))

	httpClient := &http.Client{Timeout: 70 * time.Second}
	if apiKey != "" {
		httpClient.Transport = bearerTransport{token: apiKey, base: http.DefaultTransport}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "navego-smoke", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: httpClient}, nil)
	must("connect MCP", err)
	defer session.Close()

	listed, err := session.ListTools(ctx, nil)
	must("list tools", err)
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	fmt.Printf("MCP connected: %d tools\n%s\n", len(names), strings.Join(names, ", "))

	call(ctx, session, "browser_status", map[string]any{})
	call(ctx, session, "browser_open", map[string]any{"url": targetURL})
	call(ctx, session, "browser_find", map[string]any{"query": "Example Domain", "limit": 3})
	call(ctx, session, "browser_wait", map[string]any{"text": "Example Domain", "timeout_ms": 2_000})
	call(ctx, session, "browser_scroll", map[string]any{"direction": "down", "amount": 300})
	call(ctx, session, "browser_press_key", map[string]any{"key": "HOME"})

	call(ctx, session, "browser_request_human_login", map[string]any{"reason": "smoke test of the takeover boundary"})
	call(ctx, session, "browser_snapshot", map[string]any{})
	fmt.Println("Human handoff: the next browser call reclaimed automation")
	call(ctx, session, "browser_resume_after_human", map[string]any{})
	call(ctx, session, "browser_list_tabs", map[string]any{})
	call(ctx, session, "browser_take_screenshot", map[string]any{"full_page": false})
	fmt.Println("Smoke test passed")
}

func call(ctx context.Context, session *mcp.ClientSession, name string, arguments map[string]any) {
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	must("call "+name, err)
	if result.IsError {
		panic(fmt.Sprintf("%s returned a tool error: %+v", name, result.Content))
	}
	fmt.Printf("%s: ok\n", name)
}

func must(action string, err error) {
	if err != nil {
		panic(action + ": " + err.Error())
	}
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}
