package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8001 || cfg.CDPEndpoint != "http://127.0.0.1:9222" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.ActionTimeout != 10*time.Second || cfg.SnapshotMaxElements != 150 {
		t.Fatalf("unexpected limits: %+v", cfg)
	}
}

func TestLoadRejectsUnsafeTakeoverURL(t *testing.T) {
	_, err := Load(func(name string) (string, bool) {
		if name == "HUMAN_TAKEOVER_URL" {
			return "https://user:secret@example.com", true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("expected URL validation error")
	}
}

func TestLoadOverridesValues(t *testing.T) {
	values := map[string]string{
		"MCP_PORT":                  "9001",
		"MCP_ACTION_TIMEOUT_MS":     "2500",
		"MCP_SNAPSHOT_MAX_ELEMENTS": "42",
	}
	cfg, err := Load(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9001 || cfg.ActionTimeout != 2500*time.Millisecond || cfg.SnapshotMaxElements != 42 {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
}

func TestLoadOAuthConfiguration(t *testing.T) {
	values := map[string]string{
		"MCP_OAUTH_ENABLED":          "true",
		"MCP_PUBLIC_URL":             "https://mcp.browser.lspr.dev/mcp",
		"MCP_OAUTH_ISSUER":           "https://tenant.example.com/",
		"MCP_OAUTH_AUDIENCE":         "https://mcp.browser.lspr.dev/mcp",
		"MCP_OAUTH_ALLOWED_SUBJECTS": "auth0|owner,google-oauth2|owner",
	}
	cfg, err := Load(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.OAuthEnabled || cfg.PublicURL != values["MCP_PUBLIC_URL"] || len(cfg.OAuthAllowedSubjects) != 2 {
		t.Fatalf("OAuth configuration not loaded: %+v", cfg)
	}
}

func TestLoadRejectsIncompleteOAuthConfiguration(t *testing.T) {
	_, err := Load(func(name string) (string, bool) {
		if name == "MCP_OAUTH_ENABLED" {
			return "true", true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("expected incomplete OAuth configuration to fail")
	}
}

func TestLoadRejectsOAuthWithStaticAPIKey(t *testing.T) {
	values := map[string]string{
		"MCP_OAUTH_ENABLED":          "true",
		"MCP_API_KEY":                "secret",
		"MCP_PUBLIC_URL":             "https://mcp.browser.lspr.dev/mcp",
		"MCP_OAUTH_ISSUER":           "https://tenant.example.com/",
		"MCP_OAUTH_AUDIENCE":         "https://mcp.browser.lspr.dev/mcp",
		"MCP_OAUTH_ALLOWED_SUBJECTS": "auth0|owner",
	}
	_, err := Load(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
	if err == nil {
		t.Fatal("expected OAuth and static API key to be mutually exclusive")
	}
}
