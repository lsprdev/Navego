package control

import (
	"slices"
	"testing"

	"github.com/lsprdev/Navego/internal/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestValidatedPublicMCPURL(t *testing.T) {
	tests := []struct {
		value    string
		resource string
		issuer   string
		valid    bool
	}{
		{"https://mcp.example.com/mcp", "https://mcp.example.com/mcp", "https://mcp.example.com", true},
		{"http://127.0.0.1:8090/mcp", "http://127.0.0.1:8090/mcp", "http://127.0.0.1:8090", true},
		{"http://localhost:8090/mcp/", "http://localhost:8090/mcp", "http://localhost:8090", true},
		{"http://mcp.example.com/mcp", "", "", false},
		{"https://mcp.example.com/other", "", "", false},
		{"https://mcp.example.com/mcp?token=bad", "", "", false},
	}
	for _, test := range tests {
		resource, issuer, err := validatedPublicMCPURL(test.value)
		if (err == nil) != test.valid {
			t.Fatalf("validatedPublicMCPURL(%q) error=%v", test.value, err)
		}
		if resource != test.resource || issuer != test.issuer {
			t.Fatalf("validatedPublicMCPURL(%q) = %q, %q", test.value, resource, issuer)
		}
	}
}

func TestOAuthRedirectOrigin(t *testing.T) {
	if got := oauthRedirectOrigin("https://chatgpt.com/connector_platform_oauth_redirect?state=ignored"); got != "https://chatgpt.com" {
		t.Fatalf("unexpected redirect origin %q", got)
	}
	if got := oauthRedirectOrigin("not-a-url"); got != "" {
		t.Fatalf("invalid redirect produced origin %q", got)
	}
}

func TestNormalizeScopes(t *testing.T) {
	scopes, err := normalizeScopes("browser:write browser:read browser:write")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(scopes, []string{"browser:read", "browser:write"}) {
		t.Fatalf("unexpected scopes: %#v", scopes)
	}
	if _, err := normalizeScopes("browser:write"); err == nil {
		t.Fatal("expected browser:read to be required")
	}
	if _, err := normalizeScopes("browser:read admin"); err == nil {
		t.Fatal("expected unknown scope to be rejected")
	}
}

func TestMultiBrowserToolAddsOptionalSelector(t *testing.T) {
	source := &mcp.Tool{
		Name:        "browser_open",
		Description: "Open a page.",
		InputSchema: map[string]any{
			"type":       "object",
			"required":   []string{"url"},
			"properties": map[string]any{"url": map[string]any{"type": "string"}},
		},
	}
	tool, err := multiBrowserTool(source)
	if err != nil {
		t.Fatal(err)
	}
	schema := tool.InputSchema.(map[string]any)
	properties := schema["properties"].(map[string]any)
	if _, ok := properties["browser"]; !ok {
		t.Fatal("browser selector was not added")
	}
	required := schema["required"].([]any)
	for _, value := range required {
		if value == "browser" {
			t.Fatal("browser selector must remain optional")
		}
	}
	schemes := tool.Meta["securitySchemes"].([]map[string]any)
	wantScopes := mcpserver.RequiredScopes("browser_open")
	gotScopes := schemes[0]["scopes"].([]string)
	if !slices.Equal(gotScopes, wantScopes) {
		t.Fatalf("unexpected scopes: %#v", gotScopes)
	}
}

func TestTokenHashDoesNotStoreRawToken(t *testing.T) {
	const token = "secret-access-token"
	hash := tokenHash(token)
	if hash == token || len(hash) != 64 || hash != tokenHash(token) {
		t.Fatalf("unexpected token hash %q", hash)
	}
}
