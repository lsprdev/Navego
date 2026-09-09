package control

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lsprdev/Navego/internal/oauthresource"
	"github.com/lsprdev/Navego/pb_migrations"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func TestActivityDiagnosticsSanitization(t *testing.T) {
	message := `navigate https://user:secret@example.com/private-ticket?token=query-secret#fragment: deadline exceeded; Authorization: Bearer bearer-secret; password="my password"; typed private-input; worker-secret`
	got := sanitizeDiagnostic(message, "private-input", "worker-secret")
	for _, secret := range []string{"user:secret", "private-ticket", "query-secret", "fragment", "bearer-secret", "my password", "private-input", "worker-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("leaked %q in %s", secret, got)
		}
	}
	if !strings.Contains(got, "https://example.com/") || !strings.Contains(got, "deadline exceeded") {
		t.Fatalf("lost diagnostic context: %s", got)
	}
	if got != sanitizeDiagnostic(got) {
		t.Fatalf("sanitization is not idempotent: %s", sanitizeDiagnostic(got))
	}
	if len([]rune(sanitizeDiagnostic(strings.Repeat("ç", 6000)))) > 4020 {
		t.Fatal("unbounded diagnostic")
	}
}

func TestActivityDiagnosticKinds(t *testing.T) {
	for _, test := range []struct{ message, stage, want string }{
		{"resolve sig.ifc.edu.br: lookup sig.ifc.edu.br: i/o timeout", "worker", "dns_timeout"},
		{"capture screenshot: context deadline exceeded", "worker", "timeout"},
		{"Client.Timeout exceeded while awaiting headers", "transport", "timeout"},
		{"context canceled", "transport", "canceled"},
		{"Post worker/mcp: EOF", "transport", "transport"},
		{"element is no longer visible", "worker", "tool_error"},
	} {
		if got := diagnosticKind(test.message, test.stage); got != test.want {
			t.Errorf("%s: got %s want %s", test.message, got, test.want)
		}
	}
}

func TestActivityAPIOnlyReturnsOwnerSanitizedDetails(t *testing.T) {
	app, baseURL := securityApp(t, Config{AllowedEmails: "alice@example.com,bob@example.com"})
	alice := securityUser(t, app, "alice@example.com", true)
	bob := securityUser(t, app, "bob@example.com", true)
	writeAudit(app, alice.Id, "", "mcp.browser_open", "error", map[string]any{
		"error":       "navigate https://example.com/private?token=secret: context deadline exceeded",
		"duration_ms": 3001, "stage": "worker", "password": "metadata-secret", "snapshot": "private page",
	})
	writeAudit(app, alice.Id, "", "mcp.browser_snapshot", "error", nil)
	writeAudit(app, bob.Id, "", "foreign-event", "error", map[string]any{"error": "foreign-error"})
	token, err := alice.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/api/navego/activity", nil)
	req.Header.Set("Authorization", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	for _, secret := range []string{"metadata-secret", "private page", "foreign-event", "foreign-error", "token=secret"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("activity leaked %s", secret)
		}
	}
	var events []activityResponse
	if err := json.Unmarshal(body, &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events", len(events))
	}
	for _, event := range events {
		if event.Event == "mcp.browser_snapshot" && event.Diagnostics != nil {
			t.Fatal("invented historical diagnostics")
		}
		if event.Event == "mcp.browser_open" {
			if event.Diagnostics == nil || event.Diagnostics.Kind != "timeout" || event.Diagnostics.DurationMS == nil || *event.Diagnostics.DurationMS != 3001 {
				t.Fatalf("lost legacy details: %#v", event.Diagnostics)
			}
		}
	}
	if status := securityRequest(t, http.MethodGet, baseURL+"/api/navego/activity", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("anonymous access: %d", status)
	}
}

func TestWorkerCallsPersistFailureAndDuration(t *testing.T) {
	app, _ := securityApp(t, Config{AllowedEmails: "alice@example.com"})
	user := securityUser(t, app, "alice@example.com", true)
	collection, _ := app.FindCollectionByNameOrId(pb_migrations.BrowsersCollection)
	browser := core.NewRecord(collection)
	browser.Set("owner", user.Id)
	browser.Set("name", "Principal")
	browser.Set("state", "running")
	browser.Set("worker_endpoint", "http://navego-browser-test123:8001")
	if err := app.Save(browser); err != nil {
		t.Fatal(err)
	}
	user.Set("default_browser", browser.Id)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name, message, stage, kind string
		transport                  bool
		success                    bool
	}{
		{"dns", "resolve example.com: lookup example.com: i/o timeout", "worker", "dns_timeout", false, false},
		{"screenshot", "capture screenshot: context deadline exceeded", "worker", "timeout", false, false},
		{"disconnect", "EOF", "transport", "transport", true, false},
		{"success", "private successful page contents", "worker", "", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			before, err := app.FindRecordsByFilter(pb_migrations.AuditEventsCollection, "owner = {:owner}", "", 0, 0, dbx.Params{"owner": user.Id})
			if err != nil {
				t.Fatal(err)
			}
			previousIDs := map[string]bool{}
			for _, record := range before {
				previousIDs[record.Id] = true
			}
			worker := mcp.NewServer(&mcp.Implementation{Name: "test-worker", Version: "1"}, nil)
			worker.AddTool(&mcp.Tool{Name: "browser_snapshot", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{IsError: !test.success, Content: []mcp.Content{&mcp.TextContent{Text: test.message}}, StructuredContent: map[string]any{"error_code": "INVALID_ARGUMENT", "snapshot": "must-not-be-audited"}}, nil
			})
			server := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return worker }, &mcp.StreamableHTTPOptions{Stateless: true}))
			defer server.Close()
			u, _ := url.Parse(server.URL)
			s := &multiBrowserMCP{app: app, cfg: Config{WorkerAPIKey: "worker-secret", InternalHTTP: &http.Client{Transport: savedLoginRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if test.transport {
					return nil, errors.New(test.message)
				}
				clone := r.Clone(r.Context())
				clone.URL.Scheme, clone.URL.Host = u.Scheme, u.Host
				clone.Host = u.Host
				return http.DefaultTransport.RoundTrip(clone)
			})}}}
			request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "browser_snapshot", Arguments: json.RawMessage(`{}`)}, Extra: &mcp.RequestExtra{TokenInfo: &auth.TokenInfo{UserID: user.Id, Scopes: []string{oauthresource.ScopeRead}}}}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result, err := s.callWorkerTool(ctx, request, "browser_snapshot")
			if err != nil || result == nil || result.IsError == test.success {
				t.Fatalf("unexpected result: %s %v", toolFailureText(result), err)
			}
			records, err := app.FindRecordsByFilter(pb_migrations.AuditEventsCollection, "owner = {:owner}", "-created", 0, 0, dbx.Params{"owner": user.Id})
			var added []*core.Record
			for _, record := range records {
				if !previousIDs[record.Id] {
					added = append(added, record)
				}
			}
			if err != nil || len(added) != 1 {
				t.Fatalf("missing audit: %v", err)
			}
			details := readActivityDiagnostics(added[0])
			if details == nil || details.Stage != test.stage || details.Kind != test.kind || details.DurationMS == nil {
				t.Fatalf("unexpected diagnostic: %#v", details)
			}
			if test.success {
				if details.Error != "" || details.ErrorCode != "" {
					t.Fatal("audited successful output")
				}
			} else if !strings.Contains(details.Error, test.message) {
				t.Fatalf("missing error: %s", details.Error)
			}
			if !test.success && !test.transport && details.ErrorCode != "INVALID_ARGUMENT" {
				t.Fatal("missing worker code")
			}
		})
	}
}
