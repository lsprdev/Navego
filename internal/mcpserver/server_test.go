package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lsprdev/Navego/internal/approval"
	"github.com/lsprdev/Navego/internal/browser"
	"github.com/lsprdev/Navego/internal/credentials"
	"github.com/lsprdev/Navego/internal/takeover"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeBrowser struct {
	snapshot    browser.Snapshot
	commits     int
	opens       int
	privateTabs int
	hovers      int
	keyPresses  int
	selections  int
	scrolls     int
	savedLogins int
	secretOK    bool
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
func (f *fakeBrowser) Hover(context.Context, string) (browser.Snapshot, error) {
	f.hovers++
	return f.snapshot, nil
}
func (f *fakeBrowser) PressKey(context.Context, string, string) (browser.Snapshot, error) {
	f.keyPresses++
	return f.snapshot, nil
}
func (f *fakeBrowser) SelectOption(context.Context, string, string) (browser.Snapshot, error) {
	f.selections++
	return f.snapshot, nil
}
func (f *fakeBrowser) Scroll(context.Context, string, int, string) (browser.Snapshot, error) {
	f.scrolls++
	return f.snapshot, nil
}
func (f *fakeBrowser) DescribeSavedLogin(context.Context, string, string, string) (browser.SavedLoginTarget, error) {
	return browser.SavedLoginTarget{
		URL:          f.snapshot.URL,
		RawURL:       f.snapshot.URL,
		Origin:       "https://example.com",
		Generation:   f.snapshot.Generation,
		UsernameRef:  "g1e1",
		UsernameName: "Email",
		PasswordRef:  "g1e2",
		PasswordName: "Password",
		SubmitRef:    "g1e3",
		SubmitName:   "Sign in",
	}, nil
}
func (f *fakeBrowser) CommitSavedLogin(_ context.Context, _ browser.SavedLoginTarget, username, password []byte) (browser.Snapshot, error) {
	f.savedLogins++
	f.secretOK = bytes.Equal(username, []byte("owner@example.com")) && bytes.Equal(password, []byte("test-password"))
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
	for _, name := range []string{"browser_open", "browser_snapshot", "browser_find", "browser_wait", "browser_list_tabs", "browser_new_tab", "browser_new_private_tab", "browser_switch_tab", "browser_close_tab", "browser_type", "browser_hover", "browser_press_key", "browser_select_option", "browser_scroll", "browser_export_pdf", "browser_request_human_login", "browser_resume_after_human", "browser_prepare_action", "browser_commit_action"} {
		if !names[name] {
			t.Fatalf("tool %s was not advertised", name)
		}
	}
	if len(names) != 25 {
		t.Fatalf("advertised %d tools; want 25", len(names))
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

func TestSavedLoginNeverReturnsCredentialsAndCommitsOnce(t *testing.T) {
	directory := t.TempDir()
	usernamePath := filepath.Join(directory, "username")
	passwordPath := filepath.Join(directory, "password")
	manifestPath := filepath.Join(directory, "logins.json")
	if err := os.WriteFile(usernamePath, []byte("owner@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passwordPath, []byte("test-password\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(map[string]any{
		"version": 1,
		"logins": []map[string]string{{
			"id": "example", "label": "Example account", "origin": "https://example.com",
			"username_file": usernamePath, "password_file": passwordPath,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	logins, err := credentials.Load(manifestPath, directory)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeBrowser{snapshot: browser.Snapshot{URL: "https://example.com/login", Title: "Login", Generation: 1}}
	server := New(fake, takeover.New(), approval.NewStore(time.Minute), "https://127.0.0.1:3001", nil, WithSavedLogins(logins))
	client := connectTestClient(t, server.MCP)

	prepared, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "browser_prepare_saved_login",
		Arguments: map[string]any{
			"username_ref": "g1e1", "password_ref": "g1e2", "submit_ref": "g1e3",
		},
	})
	if err != nil || prepared.IsError {
		t.Fatalf("prepare saved login: result=%+v err=%v", prepared, err)
	}
	assertResultHasNoSecrets(t, prepared)
	structured, ok := prepared.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("unexpected structured content: %#v", prepared.StructuredContent)
	}
	id, _ := structured["approval_id"].(string)
	if id == "" {
		t.Fatal("saved-login approval ID missing")
	}
	params := &mcp.CallToolParams{Name: "browser_commit_saved_login", Arguments: map[string]any{"approval_id": id}}
	committed, err := client.CallTool(t.Context(), params)
	if err != nil || committed.IsError {
		t.Fatalf("commit saved login: result=%+v err=%v", committed, err)
	}
	assertResultHasNoSecrets(t, committed)
	if fake.savedLogins != 1 || !fake.secretOK {
		t.Fatalf("saved login was not delivered exactly once: calls=%d valid=%t", fake.savedLogins, fake.secretOK)
	}
	replayed, err := client.CallTool(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.IsError || fake.savedLogins != 1 {
		t.Fatalf("saved-login replay was not blocked: result=%+v calls=%d", replayed, fake.savedLogins)
	}
}

func assertResultHasNoSecrets(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"owner@example.com", "test-password"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("tool result leaked secret %q", secret)
		}
	}
}

func TestRobustInteractionTools(t *testing.T) {
	fake := &fakeBrowser{snapshot: browser.Snapshot{URL: "https://example.com", Title: "Example", Generation: 1}}
	server := New(fake, takeover.New(), approval.NewStore(time.Minute), "https://127.0.0.1:3001", nil)
	client := connectTestClient(t, server.MCP)

	calls := []struct {
		name string
		args map[string]any
	}{
		{"browser_hover", map[string]any{"ref": "g1e1"}},
		{"browser_press_key", map[string]any{"ref": "g1e1", "key": "ESCAPE"}},
		{"browser_select_option", map[string]any{"ref": "g1e2", "option": "One"}},
		{"browser_scroll", map[string]any{"direction": "down", "amount": 500}},
	}
	for _, call := range calls {
		result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: call.name, Arguments: call.args})
		if err != nil || result.IsError {
			t.Fatalf("%s: result=%+v err=%v", call.name, result, err)
		}
	}
	if fake.hovers != 1 || fake.keyPresses != 1 || fake.selections != 1 || fake.scrolls != 1 {
		t.Fatalf("interaction calls = hover:%d key:%d select:%d scroll:%d", fake.hovers, fake.keyPresses, fake.selections, fake.scrolls)
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
