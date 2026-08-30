package control

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lsprdev/Navego/pb_migrations"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

func TestOAuthAndMultiBrowserMCPEndToEnd(t *testing.T) {
	const (
		resource    = "http://127.0.0.1:8090/mcp"
		redirectURI = "http://127.0.0.1:4545/callback"
		email       = "oauth-smoke@example.com"
		password    = "smoke-password-123"
	)
	app := New(Config{
		DataDir:                t.TempDir(),
		PublicMCPURL:           resource,
		PublicViewerURL:        "http://127.0.0.1:8090",
		PublicDashboardURL:     "http://127.0.0.1:3000",
		PublicDashboardOrigins: "http://127.0.0.1:3000",
	})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.Set("name", "OAuth Smoke")
	user.Set("email", email)
	user.Set("password", password)
	user.Set("passwordConfirm", password)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	browsers, err := app.FindCollectionByNameOrId(pb_migrations.BrowsersCollection)
	if err != nil {
		t.Fatal(err)
	}
	browser := core.NewRecord(browsers)
	browser.Set("owner", user.Id)
	browser.Set("name", "Trabalho")
	browser.Set("state", "stopped")
	browser.Set("last_title", "Chromium de teste")
	if err := app.Save(browser); err != nil {
		t.Fatal(err)
	}
	secondBrowser := core.NewRecord(browsers)
	secondBrowser.Set("owner", user.Id)
	secondBrowser.Set("name", "Pessoal")
	secondBrowser.Set("state", "stopped")
	if err := app.Save(secondBrowser); err != nil {
		t.Fatal(err)
	}
	foreignUser := core.NewRecord(users)
	foreignUser.Set("name", "Outro usuário")
	foreignUser.Set("email", "other-oauth-smoke@example.com")
	foreignUser.Set("password", password)
	foreignUser.Set("passwordConfirm", password)
	if err := app.Save(foreignUser); err != nil {
		t.Fatal(err)
	}
	foreignBrowser := core.NewRecord(browsers)
	foreignBrowser.Set("owner", foreignUser.Id)
	foreignBrowser.Set("name", "Sigiloso")
	foreignBrowser.Set("state", "running")
	if err := app.Save(foreignBrowser); err != nil {
		t.Fatal(err)
	}
	user.Set("default_browser", browser.Id)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	if err := app.OnServe().Trigger(serveEvent); err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(mux)
	defer server.Close()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	metadataResponse, err := client.Get(server.URL + "/.well-known/oauth-authorization-server")
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Issuer                      string `json:"issuer"`
		AuthorizationResponseIssuer bool   `json:"authorization_response_iss_parameter_supported"`
	}
	decodeBody(t, metadataResponse, &metadata)
	if metadata.Issuer != "http://127.0.0.1:8090" || !metadata.AuthorizationResponseIssuer {
		t.Fatalf("issuer identification metadata is incomplete: %#v", metadata)
	}

	registration := doJSONRequest(t, client, http.MethodPost, server.URL+"/oauth/register", map[string]any{
		"client_name": "Navego smoke", "redirect_uris": []string{redirectURI}, "token_endpoint_auth_method": "none",
	})
	if registration.StatusCode != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", registration.StatusCode, readBody(t, registration))
	}
	var registered struct {
		ClientID string `json:"client_id"`
	}
	decodeBody(t, registration, &registered)
	if !strings.HasPrefix(registered.ClientID, "nvg_") {
		t.Fatalf("unexpected client id %q", registered.ClientID)
	}

	verifier := strings.Repeat("v", 64)
	challengeBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])
	authorizeValues := url.Values{
		"client_id":             {registered.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {"browser:read browser:capture browser:interact browser:write browser:takeover"},
		"state":                 {"smoke-state"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"resource":              {resource},
	}
	authorizePage, err := client.Get(server.URL + "/oauth/authorize?" + authorizeValues.Encode())
	if err != nil {
		t.Fatal(err)
	}
	authorizeCSP := authorizePage.Header.Get("Content-Security-Policy")
	authorizeBody := readBody(t, authorizePage)
	if authorizePage.StatusCode != http.StatusOK || !strings.Contains(authorizeBody, "Autorizar Navego smoke") {
		t.Fatal("authorization page was not rendered")
	}
	if !strings.Contains(authorizeCSP, "form-action 'self' http://127.0.0.1:8090 http://127.0.0.1:4545") {
		t.Fatalf("authorization CSP does not allow the validated issuer: %q", authorizeCSP)
	}
	if !strings.Contains(authorizeBody, `action="http://127.0.0.1:8090/oauth/authorize"`) {
		t.Fatal("authorization form does not use the validated absolute endpoint")
	}
	authorizeValues.Set("decision", "deny")
	deniedResponse, err := client.PostForm(server.URL+"/oauth/authorize", authorizeValues)
	if err != nil {
		t.Fatal(err)
	}
	_ = deniedResponse.Body.Close()
	deniedCallback, err := url.Parse(deniedResponse.Header.Get("Location"))
	if err != nil || deniedCallback.Query().Get("error") != "access_denied" || deniedCallback.Query().Get("iss") != "http://127.0.0.1:8090" {
		t.Fatalf("denied callback does not identify the issuer: location=%q err=%v", deniedResponse.Header.Get("Location"), err)
	}
	authorizeValues.Set("email", email)
	authorizeValues.Set("password", password)
	authorizeValues.Set("decision", "approve")
	authorizeResponse, err := client.PostForm(server.URL+"/oauth/authorize", authorizeValues)
	if err != nil {
		t.Fatal(err)
	}
	defer authorizeResponse.Body.Close()
	if authorizeResponse.StatusCode != http.StatusFound {
		t.Fatalf("authorize status=%d body=%s", authorizeResponse.StatusCode, readBody(t, authorizeResponse))
	}
	callback, err := url.Parse(authorizeResponse.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if callback.Query().Get("state") != "smoke-state" || callback.Query().Get("iss") != "http://127.0.0.1:8090" {
		t.Fatalf("unexpected callback %s", callback)
	}
	code := callback.Query().Get("code")
	if code == "" {
		t.Fatal("authorization code missing")
	}

	tokenResponse, err := client.PostForm(server.URL+"/oauth/token", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {registered.ClientID},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
		"resource":      {resource},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tokenResponse.StatusCode != http.StatusOK {
		t.Fatalf("token status=%d body=%s", tokenResponse.StatusCode, readBody(t, tokenResponse))
	}
	var tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	decodeBody(t, tokenResponse, &tokens)
	if tokens.AccessToken == "" || tokens.RefreshToken == "" || !strings.Contains(tokens.Scope, "browser:write") {
		t.Fatalf("unexpected token response: %#v", tokens)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mcpHTTPClient := &http.Client{Timeout: 10 * time.Second, Transport: workerBearerTransport{token: tokens.AccessToken, base: http.DefaultTransport}}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "oauth-smoke", Version: "0.1.0"}, nil)
	session, err := mcpClient.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: server.URL + "/mcp", HTTPClient: mcpHTTPClient, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsTool(listed.Tools, "browser_list_instances") || !containsTool(listed.Tools, "browser_open") || !containsTool(listed.Tools, "browser_login_with_saved_access") {
		t.Fatal("central and worker tools were not both advertised")
	}
	if containsTool(listed.Tools, "browser_prepare_saved_login") || containsTool(listed.Tools, "browser_commit_saved_login") {
		t.Fatal("legacy file-backed saved-login tools must not be exposed by the multi-browser control plane")
	}
	var screenshotTemplateURI string
	for _, tool := range listed.Tools {
		if tool.Name != "browser_take_screenshot" {
			continue
		}
		ui, _ := tool.Meta["ui"].(map[string]any)
		screenshotTemplateURI, _ = ui["resourceUri"].(string)
		break
	}
	if screenshotTemplateURI == "" {
		t.Fatal("public screenshot tool does not advertise its MCP Apps template")
	}
	templateResult, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: screenshotTemplateURI})
	if err != nil {
		t.Fatalf("read public screenshot template: %v", err)
	}
	if len(templateResult.Contents) != 1 || templateResult.Contents[0].MIMEType != "text/html;profile=mcp-app" || !strings.Contains(templateResult.Contents[0].Text, "ui/notifications/tool-result") {
		t.Fatalf("unexpected public screenshot template: %#v", templateResult.Contents)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "browser_list_instances", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !toolResultContains(result, "Trabalho") || !toolResultContains(result, "Pessoal") || !toolResultContains(result, "padrão") {
		t.Fatalf("unexpected browser list result: %#v", result)
	}
	if toolResultContains(result, "Sigiloso") {
		t.Fatal("browser owned by another user leaked into the MCP result")
	}
	setDefaultResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "browser_set_default", Arguments: map[string]any{"browser": "Pessoal"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if setDefaultResult.IsError || !toolResultContains(setDefaultResult, "Pessoal") {
		t.Fatalf("unexpected default selection result: %#v", setDefaultResult)
	}
	user, err = app.FindRecordById("users", user.Id)
	if err != nil || user.GetString("default_browser") != secondBrowser.Id {
		t.Fatalf("default browser was not persisted: user=%#v err=%v", user, err)
	}

	refreshResponse, err := client.PostForm(server.URL+"/oauth/token", url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {tokens.RefreshToken}, "client_id": {registered.ClientID}, "resource": {resource},
	})
	if err != nil {
		t.Fatal(err)
	}
	if refreshResponse.StatusCode != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", refreshResponse.StatusCode, readBody(t, refreshResponse))
	}
	var rotated struct {
		AccessToken string `json:"access_token"`
	}
	decodeBody(t, refreshResponse, &rotated)
	if rotated.AccessToken == "" || rotated.AccessToken == tokens.AccessToken {
		t.Fatal("refresh token did not rotate the access token")
	}

	revokeResponse, err := client.PostForm(server.URL+"/oauth/revoke", url.Values{
		"token": {rotated.AccessToken}, "client_id": {registered.ClientID},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = revokeResponse.Body.Close()
	if revokeResponse.StatusCode != http.StatusOK {
		t.Fatalf("revoke status=%d", revokeResponse.StatusCode)
	}
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/mcp", nil)
	request.Header.Set("Authorization", "Bearer "+rotated.AccessToken)
	revokedResponse, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer revokedResponse.Body.Close()
	if revokedResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked access token status=%d", revokedResponse.StatusCode)
	}
}

func doJSONRequest(t *testing.T, client *http.Client, method, endpoint string, value any) *http.Response {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(method, endpoint, strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decodeBody(t *testing.T, response *http.Response, destination any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatal(err)
	}
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func containsTool(tools []*mcp.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func toolResultContains(result *mcp.CallToolResult, value string) bool {
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok && strings.Contains(text.Text, value) {
			return true
		}
	}
	return false
}
