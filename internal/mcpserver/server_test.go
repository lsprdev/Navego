package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lsprdev/Navego/internal/approval"
	"github.com/lsprdev/Navego/internal/browser"
	"github.com/lsprdev/Navego/internal/takeover"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeBrowser struct {
	snapshot    browser.Snapshot
	commits     int
	opens       int
	privateTabs int
}

func (f *fakeBrowser) Status(context.Context) (browser.Status, error) {
	return browser.Status{Connected: true, URL: f.snapshot.URL, Title: f.snapshot.Title}, nil
}
func (f *fakeBrowser) Open(context.Context, string) (browser.Snapshot, error) {
	f.opens++
	return f.snapshot, nil
}
func (f *fakeBrowser) Snapshot(context.Context) (browser.Snapshot, error) { return f.snapshot, nil }
func (f *fakeBrowser) Find(_ context.Context, query string, limit int) (browser.FindResult, error) {
	return browser.FindSnapshot(f.snapshot, query, limit)
}
func (f *fakeBrowser) Wait(context.Context, browser.WaitCondition) (browser.Snapshot, error) {
	return f.snapshot, nil
}
func (f *fakeBrowser) ListTabs(context.Context) (browser.TabsResult, error) {
	return browser.TabsResult{Tabs: []browser.Tab{{ID: "tab-1", URL: f.snapshot.URL, Title: f.snapshot.Title, Active: true}}}, nil
}
func (f *fakeBrowser) NewTab(context.Context, string) (browser.Snapshot, error) {
	return f.snapshot, nil
}
func (f *fakeBrowser) NewPrivateTab(context.Context, string) (browser.Snapshot, error) {
	f.privateTabs++
	return f.snapshot, nil
}
func (f *fakeBrowser) SwitchTab(context.Context, string) (browser.Snapshot, error) {
	return f.snapshot, nil
}
func (f *fakeBrowser) CloseTab(context.Context, string) (browser.Snapshot, error) {
	return f.snapshot, nil
}
func (f *fakeBrowser) Click(context.Context, string) (browser.Snapshot, error) {
	return f.snapshot, nil
}
func (f *fakeBrowser) Type(context.Context, string, string, bool) (browser.Snapshot, error) {
	return f.snapshot, nil
}
func (f *fakeBrowser) Screenshot(context.Context, bool) ([]byte, string, error) {
	return []byte("png"), "image/png", nil
}
func (f *fakeBrowser) PDF(context.Context) ([]byte, string, error) {
	return []byte("pdf"), "application/pdf", nil
}
func (f *fakeBrowser) DescribeAction(context.Context, string) (browser.ActionTarget, error) {
	return browser.ActionTarget{Ref: "g1e1", Role: "button", Name: "Post", URL: f.snapshot.URL, Generation: 1}, nil
}
func (f *fakeBrowser) CommitAction(context.Context, browser.ActionTarget) (browser.Snapshot, error) {
	f.commits++
	return f.snapshot, nil
}
func (f *fakeBrowser) Close() error { return nil }

func connectTestClient(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func TestAdvertisesMinimalToolsAndTakeoverBoundary(t *testing.T) {
	fake := &fakeBrowser{snapshot: browser.Snapshot{URL: "https://x.com", Title: "X", Generation: 1}}
	state := takeover.New()
	server := New(fake, state, approval.NewStore(time.Minute), "https://127.0.0.1:3001", nil)
	client := connectTestClient(t, server.MCP)
	listed, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool)
	for _, tool := range listed.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"browser_open", "browser_snapshot", "browser_find", "browser_wait", "browser_list_tabs", "browser_new_tab", "browser_new_private_tab", "browser_switch_tab", "browser_close_tab", "browser_type", "browser_export_pdf", "browser_request_human_login", "browser_resume_after_human", "browser_prepare_action", "browser_commit_action"} {
		if !names[name] {
			t.Fatalf("tool %s was not advertised", name)
		}
	}
	if len(names) != 19 {
		t.Fatalf("advertised %d tools; want 19", len(names))
	}
	found, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "browser_find", Arguments: map[string]any{"query": "X", "limit": 3}})
	if err != nil || found.IsError {
		t.Fatalf("find: result=%+v err=%v", found, err)
	}
	tabs, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "browser_list_tabs", Arguments: map[string]any{}})
	if err != nil || tabs.IsError {
		t.Fatalf("tabs: result=%+v err=%v", tabs, err)
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "browser_request_human_login", Arguments: map[string]any{"reason": "X login"}})
	if err != nil || result.IsError {
		t.Fatalf("request takeover: result=%+v err=%v", result, err)
	}
	blocked, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "browser_snapshot", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if !blocked.IsError {
		t.Fatal("snapshot should be blocked during human control")
	}
	resumed, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "browser_resume_after_human", Arguments: map[string]any{}})
	if err != nil || resumed.IsError {
		t.Fatalf("resume: result=%+v err=%v", resumed, err)
	}
}

func TestOpenAndPrivateTab(t *testing.T) {
	fake := &fakeBrowser{snapshot: browser.Snapshot{URL: "https://example.com", Title: "Example", Backend: "chromium", Generation: 1}}
	server := New(fake, takeover.New(), approval.NewStore(time.Minute), "https://127.0.0.1:3001", nil)
	client := connectTestClient(t, server.MCP)

	opened, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "browser_open",
		Arguments: map[string]any{"url": "https://example.com"},
	})
	if err != nil || opened.IsError {
		t.Fatalf("open with Chromium: result=%+v err=%v", opened, err)
	}
	if fake.opens != 1 {
		t.Fatalf("opens = %d, want 1", fake.opens)
	}

	privateTab, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "browser_new_private_tab",
		Arguments: map[string]any{"url": "https://example.com/private"},
	})
	if err != nil || privateTab.IsError {
		t.Fatalf("open private tab: result=%+v err=%v", privateTab, err)
	}
	if fake.privateTabs != 1 {
		t.Fatalf("private tabs = %d, want 1", fake.privateTabs)
	}
	if !strings.Contains(Instructions, "private") || strings.Contains(Instructions, `"ob:"`) || strings.Contains(Instructions, `"ch:"`) {
		t.Fatal("server instructions do not describe Chromium-only private browsing")
	}
}

func TestScreenshotToolProvidesInlineUI(t *testing.T) {
	fake := &fakeBrowser{snapshot: browser.Snapshot{URL: "https://example.com", Title: "Example", Generation: 1}}
	server := New(fake, takeover.New(), approval.NewStore(time.Minute), "https://127.0.0.1:3001", nil)
	client := connectTestClient(t, server.MCP)

	listed, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var screenshot *mcp.Tool
	for _, tool := range listed.Tools {
		if tool.Name == "browser_take_screenshot" {
			screenshot = tool
			break
		}
	}
	if screenshot == nil {
		t.Fatal("screenshot tool was not advertised")
	}
	ui, ok := screenshot.Meta["ui"].(map[string]any)
	if !ok || ui["resourceUri"] != screenshotUIResourceURI {
		t.Fatalf("screenshot UI metadata = %#v", screenshot.Meta["ui"])
	}
	if got := screenshot.Meta["openai/outputTemplate"]; got != screenshotUIResourceURI {
		t.Fatalf("output template = %#v, want %q", got, screenshotUIResourceURI)
	}

	resource, err := client.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: screenshotUIResourceURI})
	if err != nil {
		t.Fatal(err)
	}
	if len(resource.Contents) != 1 || resource.Contents[0].MIMEType != "text/html;profile=mcp-app" {
		t.Fatalf("screenshot UI resource = %#v", resource.Contents)
	}
	if html := resource.Contents[0].Text; !strings.Contains(html, "toolResponseMetadata") || !strings.Contains(html, "ui/notifications/tool-result") {
		t.Fatalf("screenshot UI is missing MCP Apps result handling")
	}

	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "browser_take_screenshot", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("screenshot: result=%+v err=%v", result, err)
	}
	if len(result.Content) != 2 {
		t.Fatalf("screenshot content = %#v", result.Content)
	}
	image, ok := result.Content[0].(*mcp.ImageContent)
	if !ok || image.MIMEType != "image/png" || string(image.Data) != "png" {
		t.Fatalf("screenshot image = %#v", result.Content[0])
	}
}

func TestApprovalCanCommitOnlyOnce(t *testing.T) {
	fake := &fakeBrowser{snapshot: browser.Snapshot{URL: "https://x.com/compose/post", Title: "X", Generation: 1}}
	server := New(fake, takeover.New(), approval.NewStore(time.Minute), "https://127.0.0.1:3001", nil)
	client := connectTestClient(t, server.MCP)
	prepared, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "browser_prepare_action",
		Arguments: map[string]any{"ref": "g1e1", "summary": "Post Testando automação Codex on X"},
	})
	if err != nil || prepared.IsError {
		t.Fatalf("prepare: result=%+v err=%v", prepared, err)
	}
	structured, ok := prepared.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("unexpected structured content: %#v", prepared.StructuredContent)
	}
	id, _ := structured["approval_id"].(string)
	if id == "" {
		t.Fatal("approval ID missing")
	}
	params := &mcp.CallToolParams{Name: "browser_commit_action", Arguments: map[string]any{"approval_id": id}}
	committed, err := client.CallTool(t.Context(), params)
	if err != nil || committed.IsError {
		t.Fatalf("commit: result=%+v err=%v", committed, err)
	}
	replayed, err := client.CallTool(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.IsError || fake.commits != 1 {
		t.Fatalf("replay was not blocked: result=%+v commits=%d", replayed, fake.commits)
	}
}

func TestOAuthScopesAreAdvertisedAndEnforced(t *testing.T) {
	fake := &fakeBrowser{snapshot: browser.Snapshot{URL: "https://example.com", Title: "Example", Generation: 1}}
	server := New(
		fake,
		takeover.New(),
		approval.NewStore(time.Minute),
		"https://browser.lspr.dev",
		nil,
		WithAuthorization(Authorization{
			Enabled:             true,
			ResourceMetadataURL: "https://mcp.browser.lspr.dev/.well-known/oauth-protected-resource",
		}),
	)
	client := connectTestClient(t, server.MCP)
	listed, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var screenshot *mcp.Tool
	for _, candidate := range listed.Tools {
		if candidate.Name == "browser_take_screenshot" {
			screenshot = candidate
			break
		}
	}
	if screenshot == nil {
		t.Fatal("screenshot tool was not advertised")
	}
	if _, ok := screenshot.Meta["securitySchemes"]; !ok {
		t.Fatalf("security schemes missing: %+v", screenshot.Meta)
	}

	denied, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "browser_status"})
	if err != nil {
		t.Fatal(err)
	}
	if !denied.IsError {
		t.Fatal("tool call without TokenInfo should be denied")
	}
	if _, ok := denied.Meta["mcp/www_authenticate"]; !ok {
		t.Fatalf("OAuth challenge missing: %+v", denied.Meta)
	}
}

func TestAuthorizeToolRequiresEveryScope(t *testing.T) {
	request := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{TokenInfo: &auth.TokenInfo{
		Scopes: []string{"browser:read", "browser:capture"},
	}}}
	if result := authorizeTool(request, []string{"browser:read", "browser:capture"}, "https://example.com/metadata"); result != nil {
		t.Fatalf("authorized request was denied: %+v", result)
	}
	result := authorizeTool(request, []string{"browser:read", "browser:interact"}, "https://example.com/metadata")
	if result == nil || !result.IsError {
		t.Fatalf("missing scope was allowed: %+v", result)
	}
}
