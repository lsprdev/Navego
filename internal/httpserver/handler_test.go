package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lsprdev/Navego/internal/browser"
	"github.com/lsprdev/Navego/internal/oauthresource"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type stubBrowser struct{}

func (stubBrowser) Status(context.Context) (browser.Status, error) {
	return browser.Status{Connected: true, URL: "https://example.com", Title: "Example"}, nil
}
func (stubBrowser) Open(context.Context, string) (browser.Snapshot, error) {
	return browser.Snapshot{}, nil
}
func (stubBrowser) Snapshot(context.Context) (browser.Snapshot, error) {
	return browser.Snapshot{}, nil
}
func (stubBrowser) Click(context.Context, string) (browser.Snapshot, error) {
	return browser.Snapshot{}, nil
}
func (stubBrowser) Type(context.Context, string, string, bool) (browser.Snapshot, error) {
	return browser.Snapshot{}, nil
}
func (stubBrowser) Screenshot(context.Context, bool) ([]byte, string, error) {
	return []byte("png"), "image/png", nil
}
func (stubBrowser) PDF(context.Context) ([]byte, string, error) {
	return nil, "application/pdf", nil
}
func (stubBrowser) DescribeAction(context.Context, string) (browser.ActionTarget, error) {
	return browser.ActionTarget{}, nil
}
func (stubBrowser) CommitAction(context.Context, browser.ActionTarget) (browser.Snapshot, error) {
	return browser.Snapshot{}, nil
}
func (stubBrowser) Close() error { return nil }

type savedLoginStubBrowser struct {
	stubBrowser
	target    browser.SavedLoginTarget
	username  []byte
	password  []byte
	committed bool
}

func (browserStub *savedLoginStubBrowser) DescribeSavedLogin(_ context.Context, usernameRef, passwordRef, submitRef string) (browser.SavedLoginTarget, error) {
	if usernameRef != "g1e1" || passwordRef != "g1e2" || submitRef != "g1e3" {
		return browser.SavedLoginTarget{}, errors.New("unexpected refs")
	}
	return browserStub.target, nil
}

func (browserStub *savedLoginStubBrowser) CommitSavedLogin(_ context.Context, target browser.SavedLoginTarget, username, password []byte) (browser.Snapshot, error) {
	if target.RawURL != browserStub.target.RawURL || target.Origin != browserStub.target.Origin {
		return browser.Snapshot{}, errors.New("unexpected target")
	}
	browserStub.username = append([]byte(nil), username...)
	browserStub.password = append([]byte(nil), password...)
	browserStub.committed = true
	return browser.Snapshot{URL: "https://portal.example.com/home", Title: "Portal"}, nil
}

func TestHealthDoesNotRequireAuthentication(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	handler := New(Options{MCPServer: server, Browser: stubBrowser{}, APIKey: "secret"})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestInternalScreenshotUsesWorkerBearerToken(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	handler := New(Options{MCPServer: server, Browser: stubBrowser{}, APIKey: "secret"})

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/internal/screenshot", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	request := httptest.NewRequest(http.MethodGet, "/internal/screenshot", nil)
	request.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "image/png" || recorder.Body.String() != "png" {
		t.Fatalf("unexpected screenshot response: status=%d type=%q body=%q", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
}

func TestInternalSavedLoginBrokerRequiresBearerAndNeverReturnsSecrets(t *testing.T) {
	browserStub := &savedLoginStubBrowser{target: browser.SavedLoginTarget{
		URL: "https://portal.example.com/login", RawURL: "https://portal.example.com/login?service=student",
		Origin: "https://portal.example.com", Generation: 7,
		UsernameRef: "g1e1", UsernameName: "Student ID",
		PasswordRef: "g1e2", PasswordName: "Password",
		SubmitRef: "g1e3", SubmitName: "Sign in",
	}}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	handler := New(Options{MCPServer: server, Browser: browserStub, APIKey: "worker-secret"})

	describeBody, err := json.Marshal(InternalSavedLoginDescribeRequest{
		UsernameRef: "g1e1", PasswordRef: "g1e2", SubmitRef: "g1e3",
	})
	if err != nil {
		t.Fatal(err)
	}
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/internal/saved-login/describe", bytes.NewReader(describeBody)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	describeRequest := httptest.NewRequest(http.MethodPost, "/internal/saved-login/describe", bytes.NewReader(describeBody))
	describeRequest.Header.Set("Authorization", "Bearer worker-secret")
	described := httptest.NewRecorder()
	handler.ServeHTTP(described, describeRequest)
	if described.Code != http.StatusOK {
		t.Fatalf("describe status = %d body=%s", described.Code, described.Body.String())
	}
	var description InternalSavedLoginDescribeResponse
	if err := json.Unmarshal(described.Body.Bytes(), &description); err != nil {
		t.Fatal(err)
	}
	if description.Target.RawURL != browserStub.target.RawURL || description.Target.Origin != browserStub.target.Origin {
		t.Fatalf("unexpected target: %#v", description.Target)
	}

	commitBody, err := json.Marshal(InternalSavedLoginCommitRequest{
		Target: description.Target, Username: []byte("student-123"), Password: []byte("vault-password"),
	})
	if err != nil {
		t.Fatal(err)
	}
	commitRequest := httptest.NewRequest(http.MethodPost, "/internal/saved-login/commit", bytes.NewReader(commitBody))
	commitRequest.Header.Set("Authorization", "Bearer worker-secret")
	committed := httptest.NewRecorder()
	handler.ServeHTTP(committed, commitRequest)
	if committed.Code != http.StatusOK || !browserStub.committed {
		t.Fatalf("commit status = %d body=%s committed=%t", committed.Code, committed.Body.String(), browserStub.committed)
	}
	if string(browserStub.username) != "student-123" || string(browserStub.password) != "vault-password" {
		t.Fatal("worker did not receive the expected credential bytes")
	}
	if strings.Contains(committed.Body.String(), "student-123") || strings.Contains(committed.Body.String(), "vault-password") {
		t.Fatalf("internal commit response leaked credential material: %s", committed.Body.String())
	}
}

func TestInternalSavedLoginBrokerFailsClosedWithoutWorkerKey(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	handler := New(Options{MCPServer: server, Browser: stubBrowser{}})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/internal/saved-login/describe", strings.NewReader(`{}`)))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestUnknownDiscoveryPathsReturnNotFoundWithoutOAuth(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	handler := New(Options{MCPServer: server, Browser: stubBrowser{}})

	for _, path := range []string{
		"/.well-known/oauth-protected-resource",
		"/.well-known/oauth-protected-resource/mcp",
		"/.well-known/oauth-authorization-server",
		"/.well-known/openid-configuration",
		"/mcp/.well-known/oauth-protected-resource",
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("unknown discovery path %s status = %d, want %d", path, recorder.Code, http.StatusNotFound)
		}
	}

	root := httptest.NewRecorder()
	handler.ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/", nil))
	if root.Code != http.StatusOK {
		t.Fatalf("root status = %d, want %d", root.Code, http.StatusOK)
	}
}

func TestMCPAdvertisesCurrentStatelessProtocol(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	handler := New(Options{MCPServer: server, Browser: stubBrowser{}})
	body := `{"jsonrpc":"2.0","id":"openai-mcp-discover","method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"openai-mcp","version":"1.0.0"},"io.modelcontextprotocol/clientCapabilities":{}}}}`
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Mcp-Method", "server/discover")
	request.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("server/discover status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		Result struct {
			SupportedVersions []string `json:"supportedVersions"`
		} `json:"result"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, version := range response.Result.SupportedVersions {
		if version == "2026-07-28" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("supportedVersions = %v, want 2026-07-28", response.Result.SupportedVersions)
	}
	if sessionID := recorder.Header().Get("Mcp-Session-Id"); sessionID != "" {
		t.Fatalf("Mcp-Session-Id = %q, want stateless response", sessionID)
	}
}

func TestMCPRequiresConfiguredBearerToken(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	handler := New(Options{MCPServer: server, Browser: stubBrowser{}, APIKey: "secret"})

	for _, tc := range []struct {
		name   string
		token  string
		status int
	}{
		{name: "missing", status: http.StatusUnauthorized},
		{name: "wrong", token: "wrong", status: http.StatusUnauthorized},
		{name: "valid", token: "secret", status: http.StatusUnsupportedMediaType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			if recorder.Code != tc.status {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.status)
			}
		})
	}
}

func TestOAuthProtectsMCPAndPublishesBothMetadataLocations(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	verifier := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if token != "valid" {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{
			Scopes:     []string{oauthresource.ScopeRead},
			Expiration: time.Now().Add(time.Hour),
			UserID:     "auth0|owner",
		}, nil
	}
	handler := New(Options{
		MCPServer: server,
		Browser:   stubBrowser{},
		OAuth: &OAuthOptions{
			PublicURL:           "https://mcp.browser.lspr.dev/mcp",
			AuthorizationServer: "https://tenant.example.com/",
			TokenVerifier:       verifier,
		},
	})

	for _, path := range []string{
		"/.well-known/oauth-protected-resource",
		"/.well-known/oauth-protected-resource/mcp",
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("metadata %s status = %d, want 200", path, recorder.Code)
		}
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want 401", missing.Code)
	}
	wantChallenge := `resource_metadata="https://mcp.browser.lspr.dev/.well-known/oauth-protected-resource"`
	if got := missing.Header().Get("WWW-Authenticate"); !strings.Contains(got, wantChallenge) || !strings.Contains(got, `scope="browser:read"`) {
		t.Fatalf("WWW-Authenticate = %q", got)
	}

	validRequest := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	validRequest.Header.Set("Authorization", "Bearer valid")
	valid := httptest.NewRecorder()
	handler.ServeHTTP(valid, validRequest)
	if valid.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("valid token status = %d, want %d", valid.Code, http.StatusUnsupportedMediaType)
	}
}

func TestAdvertiseToolSecuritySchemesCopiesMetaToTopLevel(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"jsonrpc": "2.0",
			"id":      7,
			"result": map[string]any{
				"tools": []any{map[string]any{
					"name": "browser_take_screenshot",
					"_meta": map[string]any{
						"securitySchemes": []any{map[string]any{
							"type":   "oauth2",
							"scopes": []any{"browser:read", "browser:capture"},
						}},
					},
				}},
			},
		})
	})
	handler := advertiseToolSecuritySchemes(next, nilLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/list"}`))
	handler.ServeHTTP(recorder, request)

	var response struct {
		Result struct {
			Tools []map[string]any `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Result.Tools) != 1 {
		t.Fatalf("tools = %v", response.Result.Tools)
	}
	if _, ok := response.Result.Tools[0]["securitySchemes"]; !ok {
		t.Fatalf("top-level securitySchemes missing: %v", response.Result.Tools[0])
	}
	if _, ok := response.Result.Tools[0]["_meta"]; !ok {
		t.Fatalf("compatibility _meta was removed: %v", response.Result.Tools[0])
	}
}

func nilLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
