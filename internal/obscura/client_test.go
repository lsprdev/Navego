package obscura

import (
	"context"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeSession struct {
	name       string
	tools      []*mcp.Tool
	calls      []string
	closed     bool
	callResult map[string]*mcp.CallToolResult
}

func completeFakeSession() *fakeSession {
	tools := make([]*mcp.Tool, 0, len(requiredTools))
	for name, required := range requiredTools {
		properties := make(map[string]any)
		for _, property := range required {
			properties[property] = map[string]any{"type": "string"}
		}
		tools = append(tools, &mcp.Tool{
			Name:        name,
			InputSchema: map[string]any{"type": "object", "properties": properties},
		})
	}
	return &fakeSession{
		name:  "obscura-mcp",
		tools: tools,
		callResult: map[string]*mcp.CallToolResult{
			"browser_navigate":   {Content: []mcp.Content{&mcp.TextContent{Text: "Navigated"}}},
			"browser_snapshot":   {Content: []mcp.Content{&mcp.TextContent{Text: "URL: https://93.184.216.34/\nTitle: Example\n\nBody"}}},
			"browser_markdown":   {Content: []mcp.Content{&mcp.TextContent{Text: "# Example"}}},
			"browser_links":      {Content: []mcp.Content{&mcp.TextContent{Text: "{\"text\":\"More\",\"href\":\"https://iana.org\"}\n"}}},
			"browser_screenshot": {Content: []mcp.Content{&mcp.ImageContent{MIMEType: "image/png", Data: []byte("png")}}},
			"browser_pdf":        {Content: []mcp.Content{&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{MIMEType: "application/pdf", Blob: []byte("pdf")}}}},
		},
	}
}

func (f *fakeSession) CallTool(_ context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	f.calls = append(f.calls, params.Name)
	return f.callResult[params.Name], nil
}

func (f *fakeSession) ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	return &mcp.ListToolsResult{Tools: f.tools}, nil
}

func (f *fakeSession) Ping(context.Context, *mcp.PingParams) error { return nil }

func (f *fakeSession) InitializeResult() *mcp.InitializeResult {
	return &mcp.InitializeResult{ServerInfo: &mcp.Implementation{Name: f.name, Version: "0.1.0"}}
}

func (f *fakeSession) Close() error {
	f.closed = true
	return nil
}

func TestClientUsesOnlyAllowedReadTools(t *testing.T) {
	t.Parallel()
	fake := completeFakeSession()
	client := newClient(func(context.Context) (session, error) { return fake, nil }, 1<<20)

	snapshot, err := client.Open(t.Context(), "https://93.184.216.34", "load", 2_000)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.URL != "https://93.184.216.34/" || snapshot.Title != "Example" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	markdown, err := client.Markdown(t.Context(), 2_000)
	if err != nil || markdown != "# Example" {
		t.Fatalf("markdown=%q err=%v", markdown, err)
	}
	links, err := client.Links(t.Context(), false, 10)
	if err != nil || !reflect.DeepEqual(links, []Link{{Text: "More", Href: "https://iana.org"}}) {
		t.Fatalf("links=%+v err=%v", links, err)
	}
	image, imageType, err := client.Screenshot(t.Context(), 1024, 768)
	if err != nil || string(image) != "png" || imageType != "image/png" {
		t.Fatalf("screenshot=%q type=%q err=%v", image, imageType, err)
	}
	pdf, pdfType, err := client.PDF(t.Context())
	if err != nil || string(pdf) != "pdf" || pdfType != "application/pdf" {
		t.Fatalf("pdf=%q type=%q err=%v", pdf, pdfType, err)
	}

	wantCalls := []string{"browser_navigate", "browser_snapshot", "browser_markdown", "browser_links", "browser_screenshot", "browser_pdf"}
	if !reflect.DeepEqual(fake.calls, wantCalls) {
		t.Fatalf("calls=%v; want %v", fake.calls, wantCalls)
	}
	if _, err := client.callLocked(t.Context(), "browser_evaluate", map[string]any{}); err == nil {
		t.Fatal("browser_evaluate must never pass the adapter allowlist")
	}
}

func TestClientRejectsContractMismatch(t *testing.T) {
	t.Parallel()
	fake := completeFakeSession()
	fake.name = "not-obscura"
	client := newClient(func(context.Context) (session, error) { return fake, nil }, 1<<20)
	if _, err := client.Status(t.Context()); err == nil {
		t.Fatal("expected server identity mismatch")
	}
	if !fake.closed {
		t.Fatal("mismatched session should be closed")
	}

	fake = completeFakeSession()
	fake.tools = fake.tools[1:]
	client = newClient(func(context.Context) (session, error) { return fake, nil }, 1<<20)
	if _, err := client.Status(t.Context()); err == nil {
		t.Fatal("expected missing tool to fail the contract")
	}
}

func TestClientBoundsOutputs(t *testing.T) {
	t.Parallel()
	fake := completeFakeSession()
	client := newClient(func(context.Context) (session, error) { return fake, nil }, 2)
	if _, err := client.Markdown(t.Context(), 100); err == nil {
		t.Fatal("expected oversized text to fail")
	}
	if _, err := client.Links(t.Context(), false, 201); err == nil {
		t.Fatal("expected oversized link limit to fail")
	}
	if _, _, err := client.Screenshot(t.Context(), 5000, 5000); err == nil {
		t.Fatal("expected oversized screenshot dimensions to fail")
	}
}
