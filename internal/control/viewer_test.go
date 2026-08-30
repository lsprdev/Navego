package control

import (
	"net/http"
	"testing"
	"time"
)

func TestViewerTicketIsSingleUse(t *testing.T) {
	store := newViewerStore()
	ticket, err := store.mint("http://navego-browser-browser1:3000")
	if err != nil {
		t.Fatal(err)
	}
	session, grant, ok := store.consume(ticket)
	if !ok || session == "" || grant.endpoint != "http://navego-browser-browser1:3000" {
		t.Fatalf("unexpected grant: session=%q grant=%#v ok=%v", session, grant, ok)
	}
	if _, _, ok := store.consume(ticket); ok {
		t.Fatal("ticket was accepted twice")
	}
	if resolved, ok := store.resolve(session); !ok || resolved.endpoint != grant.endpoint {
		t.Fatalf("session was not resolved: %#v %v", resolved, ok)
	}
}

func TestViewerTicketExpires(t *testing.T) {
	store := newViewerStore()
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	ticket, err := store.mint("http://navego-browser-browser1:3000")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(61 * time.Second)
	if _, _, ok := store.consume(ticket); ok {
		t.Fatal("expired ticket was accepted")
	}
}

func TestInternalViewerEndpointsAreRestricted(t *testing.T) {
	valid := []string{
		"http://navego-browser-browser1:3000",
		"http://navego-browser-a_b-2:3000",
	}
	for _, value := range valid {
		if _, err := validatedViewerEndpoint(value); err != nil {
			t.Fatalf("expected %q to be valid: %v", value, err)
		}
	}
	invalid := []string{
		"https://example.com:3000",
		"http://navego-browser-browser1:8001",
		"http://127.0.0.1:3000",
		"http://navego-browser-browser1:3000/path",
	}
	for _, value := range invalid {
		if _, err := validatedViewerEndpoint(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestViewerEmbeddingReplacesFrameOptions(t *testing.T) {
	header := make(http.Header)
	header.Set("X-Frame-Options", "SAMEORIGIN")
	if err := allowViewerEmbedding(header, "http://127.0.0.1:3000, http://localhost:3000"); err != nil {
		t.Fatal(err)
	}
	if value := header.Get("X-Frame-Options"); value != "" {
		t.Fatalf("X-Frame-Options was not removed: %q", value)
	}
	if value := header.Get("Content-Security-Policy"); value != "frame-ancestors 'self' http://127.0.0.1:3000 http://localhost:3000" {
		t.Fatalf("unexpected frame policy: %q", value)
	}
}

func TestPublicDashboardOriginsAreValidatedAndDeduplicated(t *testing.T) {
	origins, err := validatedPublicOrigins("http://localhost:3000, https://browser.lspr.dev/, http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	if len(origins) != 2 || origins[0] != "http://localhost:3000" || origins[1] != "https://browser.lspr.dev" {
		t.Fatalf("unexpected origins: %#v", origins)
	}
	if _, err := validatedPublicOrigins(" , "); err == nil {
		t.Fatal("expected an empty origins list to be rejected")
	}
}

func TestPublicDashboardOriginRejectsPathsAndCredentials(t *testing.T) {
	valid := []string{"http://127.0.0.1:3000", "https://browser.lspr.dev/"}
	for _, value := range valid {
		if _, err := validatedPublicOrigin(value); err != nil {
			t.Fatalf("expected %q to be valid: %v", value, err)
		}
	}
	invalid := []string{"https://browser.lspr.dev/path", "https://user@example.com", "javascript:alert(1)"}
	for _, value := range invalid {
		if _, err := validatedPublicOrigin(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestPublicDashboardURLRejectsPathsAndCredentials(t *testing.T) {
	if value, err := validatedPublicDashboardURL("https://browser.lspr.dev/"); err != nil || value != "https://browser.lspr.dev" {
		t.Fatalf("expected dashboard URL to be normalized: %q %v", value, err)
	}
	for _, value := range []string{"https://browser.lspr.dev/takeover", "https://user@browser.lspr.dev", "javascript:alert(1)"} {
		if _, err := validatedPublicDashboardURL(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}
