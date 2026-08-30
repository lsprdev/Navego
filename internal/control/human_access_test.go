package control

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pocketbase/pocketbase/core"
)

func TestHumanAccessCanBeResolvedMoreThanOnce(t *testing.T) {
	store := newHumanAccessStore()
	ticket, err := store.mint("owner-1", "browser-1")
	if err != nil {
		t.Fatal(err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		grant, ok := store.resolve(ticket)
		if !ok || grant.ownerID != "owner-1" || grant.browserID != "browser-1" {
			t.Fatalf("attempt %d returned an unexpected grant: %#v %v", attempt, grant, ok)
		}
	}
}

func TestHumanAccessExpires(t *testing.T) {
	store := newHumanAccessStore()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	ticket, err := store.mint("owner-1", "browser-1")
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(humanAccessTTL + time.Second)
	if _, ok := store.resolve(ticket); ok {
		t.Fatal("expired human access was accepted")
	}
}

func TestHumanAccessRejectsAnotherOwner(t *testing.T) {
	store := newHumanAccessStore()
	ticket, err := store.mint("owner-1", "browser-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.resolveForOwner(ticket, "owner-2"); err != errHumanAccessMismatch {
		t.Fatalf("expected owner mismatch, got %v", err)
	}
	if grant, err := store.resolveForOwner(ticket, "owner-1"); err != nil || grant.browserID != "browser-1" {
		t.Fatalf("owner could not resolve its access: %#v %v", grant, err)
	}
}

func TestHumanLoginResultLinksToAuthenticatedDashboard(t *testing.T) {
	store := newHumanAccessStore()
	service := &multiBrowserMCP{
		cfg:         Config{PublicDashboardURL: "https://browser.example.com"},
		humanAccess: store,
	}
	collection := core.NewBaseCollection("browsers")
	collection.Fields.Add(&core.TextField{Name: "name"})
	browser := core.NewRecord(collection)
	browser.Id = "browser-1"
	browser.Set("name", "Main")
	result := &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: "private worker URL"}},
		StructuredContent: map[string]any{"status": "human_action_required"},
	}

	if err := service.replaceHumanLoginURL(result, browser, "owner-1"); err != nil {
		t.Fatal(err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("unexpected structured result: %#v", result.StructuredContent)
	}
	publicURL, _ := structured["url"].(string)
	parsed, err := url.Parse(publicURL)
	if err != nil || parsed.Host != "browser.example.com" || !strings.HasPrefix(parsed.Path, "/takeover/") {
		t.Fatalf("unexpected public takeover URL %q: %v", publicURL, err)
	}
	ticket := strings.TrimPrefix(parsed.Path, "/takeover/")
	grant, err := store.resolveForOwner(ticket, "owner-1")
	if err != nil || grant.browserID != browser.Id {
		t.Fatalf("takeover did not preserve owner/browser: %#v %v", grant, err)
	}
	if text, ok := result.Content[0].(*mcp.TextContent); !ok || !strings.Contains(text.Text, publicURL) {
		t.Fatalf("model-visible result does not contain the dashboard link: %#v", result.Content)
	}
}
