package control

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/lsprdev/Navego/internal/oauthresource"
	"github.com/lsprdev/Navego/pb_migrations"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const (
	oauthCodeTTL    = 5 * time.Minute
	accessTokenTTL  = time.Hour
	refreshTokenTTL = 30 * 24 * time.Hour
)

type oauthService struct {
	app      core.App
	resource string
	issuer   string
}

type authorizationRequest struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	Scope               string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Resource            string
}

func newOAuthService(app core.App, publicMCPURL string) (*oauthService, error) {
	resource, issuer, err := validatedPublicMCPURL(publicMCPURL)
	if err != nil {
		return nil, err
	}
	return &oauthService{app: app, resource: resource, issuer: issuer}, nil
}

func registerOAuthRoutes(event *core.ServeEvent, service *oauthService, setupErr error) {
	unavailable := func(request *core.RequestEvent) error {
		return request.InternalServerError("OAuth do Navego não está configurado.", setupErr)
	}
	if setupErr != nil {
		for _, path := range []string{
			"/.well-known/oauth-protected-resource",
			"/.well-known/oauth-protected-resource/mcp",
			"/.well-known/oauth-authorization-server",
			"/oauth/register",
			"/oauth/authorize",
			"/oauth/token",
			"/oauth/revoke",
		} {
			event.Router.Any(path, unavailable)
		}
		return
	}

	metadata := service.protectedResourceMetadata
	event.Router.GET("/.well-known/oauth-protected-resource", metadata)
	event.Router.OPTIONS("/.well-known/oauth-protected-resource", metadata)
	event.Router.GET("/.well-known/oauth-protected-resource/mcp", metadata)
	event.Router.OPTIONS("/.well-known/oauth-protected-resource/mcp", metadata)
	event.Router.GET("/.well-known/oauth-authorization-server", service.authorizationServerMetadata)
	event.Router.POST("/oauth/register", service.registerClient)
	event.Router.GET("/oauth/authorize", service.showAuthorization)
	event.Router.POST("/oauth/authorize", service.approveAuthorization)
	event.Router.POST("/oauth/token", service.exchangeToken)
	event.Router.POST("/oauth/revoke", service.revokeToken)
}

func (s *oauthService) protectedResourceMetadata(event *core.RequestEvent) error {
	event.Response.Header().Set("Cache-Control", "public, max-age=300")
	return event.JSON(http.StatusOK, map[string]any{
		"resource":                 s.resource,
		"authorization_servers":    []string{s.issuer},
		"scopes_supported":         oauthresource.AllScopes,
		"bearer_methods_supported": []string{"header"},
		"resource_name":            "Navego",
	})
}

func (s *oauthService) authorizationServerMetadata(event *core.RequestEvent) error {
	event.Response.Header().Set("Cache-Control", "public, max-age=300")
	return event.JSON(http.StatusOK, map[string]any{
		"issuer": s.issuer,
		"authorization_response_iss_parameter_supported": true,
		"authorization_endpoint":                         s.issuer + "/oauth/authorize",
		"token_endpoint":                                 s.issuer + "/oauth/token",
		"registration_endpoint":                          s.issuer + "/oauth/register",
		"revocation_endpoint":                            s.issuer + "/oauth/revoke",
		"response_types_supported":                       []string{"code"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"scopes_supported":                               oauthresource.AllScopes,
	})
}

func (s *oauthService) registerClient(event *core.RequestEvent) error {
	var input struct {
		ClientName              string   `json:"client_name"`
		RedirectURIs            []string `json:"redirect_uris"`
		TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	}
	if err := event.BindBody(&input); err != nil {
		return oauthJSONError(event, http.StatusBadRequest, "invalid_client_metadata", "Metadados do cliente inválidos.")
	}
	input.ClientName = strings.TrimSpace(input.ClientName)
	if input.ClientName == "" {
		input.ClientName = "ChatGPT"
	}
	if len([]rune(input.ClientName)) > 200 || len(input.RedirectURIs) == 0 || len(input.RedirectURIs) > 10 {
		return oauthJSONError(event, http.StatusBadRequest, "invalid_client_metadata", "Nome ou redirect_uris inválidos.")
	}
	if input.TokenEndpointAuthMethod == "" {
		input.TokenEndpointAuthMethod = "none"
	}
	if input.TokenEndpointAuthMethod != "none" {
		return oauthJSONError(event, http.StatusBadRequest, "invalid_client_metadata", "Somente clientes públicos com PKCE são aceitos.")
	}
	redirects := make([]string, 0, len(input.RedirectURIs))
	seen := map[string]struct{}{}
	for _, raw := range input.RedirectURIs {
		redirect, err := validatedRedirectURI(raw)
		if err != nil {
			return oauthJSONError(event, http.StatusBadRequest, "invalid_redirect_uri", err.Error())
		}
		if _, exists := seen[redirect]; !exists {
			seen[redirect] = struct{}{}
			redirects = append(redirects, redirect)
		}
	}

	clientIDToken, err := randomToken()
	if err != nil {
		return event.InternalServerError("Não foi possível registrar o cliente OAuth.", err)
	}
	clientID := "nvg_" + clientIDToken
	collection, err := s.app.FindCollectionByNameOrId(pb_migrations.OAuthClientsCollection)
	if err != nil {
		return event.InternalServerError("Cadastro OAuth indisponível.", err)
	}
	record := core.NewRecord(collection)
	record.Set("client_id", clientID)
	record.Set("client_name", input.ClientName)
	record.Set("redirect_uris", redirects)
	record.Set("token_endpoint_auth_method", "none")
	if err := s.app.Save(record); err != nil {
		return event.InternalServerError("Não foi possível registrar o cliente OAuth.", err)
	}
	return event.JSON(http.StatusCreated, map[string]any{
		"client_id":                  clientID,
		"client_id_issued_at":        time.Now().UTC().Unix(),
		"client_name":                input.ClientName,
		"redirect_uris":              redirects,
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
	})
}

func (s *oauthService) showAuthorization(event *core.RequestEvent) error {
	request := authorizationFromValues(event.Request.URL.Query())
	if request.Resource == "" {
		request.Resource = s.resource
	}
	client, scopes, err := s.validateAuthorizationRequest(request)
	if err != nil {
		return renderOAuthError(event, err.Error())
	}
	return renderAuthorizationPage(event, authorizationPageData{
		Request:        request,
		ClientName:     client.GetString("client_name"),
		Scopes:         scopes,
		FormAction:     s.issuer + "/oauth/authorize",
		Issuer:         s.issuer,
		RedirectOrigin: oauthRedirectOrigin(request.RedirectURI),
	})
}

func (s *oauthService) approveAuthorization(event *core.RequestEvent) error {
	if err := event.Request.ParseForm(); err != nil {
		return renderOAuthError(event, "Não foi possível ler a autorização.")
	}
	request := authorizationFromValues(event.Request.PostForm)
	if request.Resource == "" {
		request.Resource = s.resource
	}
	client, scopes, err := s.validateAuthorizationRequest(request)
	if err != nil {
		return renderOAuthError(event, err.Error())
	}
	if event.Request.PostFormValue("decision") != "approve" {
		return redirectOAuthError(event, request.RedirectURI, request.State, s.issuer, "access_denied", "A autorização foi cancelada.")
	}

	email := strings.TrimSpace(event.Request.PostFormValue("email"))
	password := event.Request.PostFormValue("password")
	user, err := s.app.FindAuthRecordByEmail("users", email)
	if err != nil || !user.ValidatePassword(password) {
		return renderAuthorizationPage(event, authorizationPageData{
			Request:        request,
			ClientName:     client.GetString("client_name"),
			Scopes:         scopes,
			Email:          email,
			Error:          "E-mail ou senha inválidos.",
			FormAction:     s.issuer + "/oauth/authorize",
			Issuer:         s.issuer,
			RedirectOrigin: oauthRedirectOrigin(request.RedirectURI),
		})
	}

	code, err := randomToken()
	if err != nil {
		return event.InternalServerError("Não foi possível emitir a autorização.", err)
	}
	collection, err := s.app.FindCollectionByNameOrId(pb_migrations.OAuthGrantsCollection)
	if err != nil {
		return event.InternalServerError("Autorizações OAuth indisponíveis.", err)
	}
	record := core.NewRecord(collection)
	record.Set("owner", user.Id)
	record.Set("client_id", request.ClientID)
	record.Set("scopes", strings.Join(scopes, " "))
	record.Set("code_hash", tokenHash(code))
	record.Set("code_challenge", request.CodeChallenge)
	record.Set("redirect_uri", request.RedirectURI)
	record.Set("resource", request.Resource)
	record.Set("expires_at", time.Now().UTC().Add(oauthCodeTTL))
	if err := s.app.Save(record); err != nil {
		return event.InternalServerError("Não foi possível salvar a autorização.", err)
	}
	writeAudit(s.app, user.Id, "", "oauth.authorize", "success", map[string]any{"client": request.ClientID})

	redirect, _ := url.Parse(request.RedirectURI)
	query := redirect.Query()
	query.Set("code", code)
	if request.State != "" {
		query.Set("state", request.State)
	}
	query.Set("iss", s.issuer)
	redirect.RawQuery = query.Encode()
	return event.Redirect(http.StatusFound, redirect.String())
}

func (s *oauthService) validateAuthorizationRequest(request authorizationRequest) (*core.Record, []string, error) {
	if request.ResponseType != "code" {
		return nil, nil, fmt.Errorf("response_type deve ser code")
	}
	if request.ClientID == "" || request.RedirectURI == "" {
		return nil, nil, fmt.Errorf("client_id e redirect_uri são obrigatórios")
	}
	if request.CodeChallengeMethod != "S256" || len(request.CodeChallenge) < 43 || len(request.CodeChallenge) > 128 {
		return nil, nil, fmt.Errorf("PKCE S256 é obrigatório")
	}
	if request.Resource == "" {
		request.Resource = s.resource
	}
	if request.Resource != s.resource {
		return nil, nil, fmt.Errorf("resource não corresponde ao MCP do Navego")
	}
	client, err := s.findClient(request.ClientID)
	if err != nil || !clientAllowsRedirect(client, request.RedirectURI) {
		return nil, nil, fmt.Errorf("cliente OAuth ou redirect_uri não reconhecido")
	}
	scopes, err := normalizeScopes(request.Scope)
	if err != nil {
		return nil, nil, err
	}
	return client, scopes, nil
}

func (s *oauthService) exchangeToken(event *core.RequestEvent) error {
	event.Response.Header().Set("Cache-Control", "no-store")
	event.Response.Header().Set("Pragma", "no-cache")
	if err := event.Request.ParseForm(); err != nil {
		return oauthJSONError(event, http.StatusBadRequest, "invalid_request", "Formulário inválido.")
	}
	switch event.Request.PostFormValue("grant_type") {
	case "authorization_code":
		return s.exchangeAuthorizationCode(event)
	case "refresh_token":
		return s.exchangeRefreshToken(event)
	default:
		return oauthJSONError(event, http.StatusBadRequest, "unsupported_grant_type", "grant_type não suportado.")
	}
}

func (s *oauthService) exchangeAuthorizationCode(event *core.RequestEvent) error {
	code := strings.TrimSpace(event.Request.PostFormValue("code"))
	clientID := strings.TrimSpace(event.Request.PostFormValue("client_id"))
	redirectURI := strings.TrimSpace(event.Request.PostFormValue("redirect_uri"))
	verifier := strings.TrimSpace(event.Request.PostFormValue("code_verifier"))
	if code == "" || clientID == "" || redirectURI == "" || len(verifier) < 43 || len(verifier) > 128 {
		return oauthJSONError(event, http.StatusBadRequest, "invalid_request", "code, client_id, redirect_uri e code_verifier são obrigatórios.")
	}
	if _, err := s.findClient(clientID); err != nil {
		return oauthJSONError(event, http.StatusUnauthorized, "invalid_client", "Cliente OAuth desconhecido.")
	}
	record, err := s.app.FindFirstRecordByFilter(pb_migrations.OAuthGrantsCollection, "code_hash = {:hash}", dbx.Params{"hash": tokenHash(code)})
	if err != nil || record.GetString("client_id") != clientID || record.GetString("redirect_uri") != redirectURI {
		return oauthJSONError(event, http.StatusBadRequest, "invalid_grant", "Código inválido.")
	}
	if !record.GetDateTime("used_at").IsZero() || !record.GetDateTime("expires_at").Time().After(time.Now().UTC()) {
		return oauthJSONError(event, http.StatusBadRequest, "invalid_grant", "Código expirado ou já utilizado.")
	}
	challenge := sha256.Sum256([]byte(verifier))
	if base64.RawURLEncoding.EncodeToString(challenge[:]) != record.GetString("code_challenge") {
		return oauthJSONError(event, http.StatusBadRequest, "invalid_grant", "Verificação PKCE inválida.")
	}
	resource := record.GetString("resource")
	if requested := strings.TrimSpace(event.Request.PostFormValue("resource")); requested != "" && requested != resource {
		return oauthJSONError(event, http.StatusBadRequest, "invalid_target", "resource inválido.")
	}
	record.Set("used_at", time.Now().UTC())
	if err := s.app.Save(record); err != nil {
		return event.InternalServerError("Não foi possível consumir o código OAuth.", err)
	}
	return s.issueTokens(event, record.GetString("owner"), clientID, strings.Fields(record.GetString("scopes")), resource)
}

func (s *oauthService) exchangeRefreshToken(event *core.RequestEvent) error {
	refreshToken := strings.TrimSpace(event.Request.PostFormValue("refresh_token"))
	clientID := strings.TrimSpace(event.Request.PostFormValue("client_id"))
	if refreshToken == "" || clientID == "" {
		return oauthJSONError(event, http.StatusBadRequest, "invalid_request", "refresh_token e client_id são obrigatórios.")
	}
	record, err := s.app.FindFirstRecordByFilter(pb_migrations.OAuthRefreshCollection, "token_hash = {:hash}", dbx.Params{"hash": tokenHash(refreshToken)})
	if err != nil || record.GetString("client_id") != clientID || !record.GetDateTime("revoked_at").IsZero() || !record.GetDateTime("expires_at").Time().After(time.Now().UTC()) {
		return oauthJSONError(event, http.StatusBadRequest, "invalid_grant", "Refresh token inválido ou expirado.")
	}
	resource := record.GetString("resource")
	if requested := strings.TrimSpace(event.Request.PostFormValue("resource")); requested != "" && requested != resource {
		return oauthJSONError(event, http.StatusBadRequest, "invalid_target", "resource inválido.")
	}
	record.Set("revoked_at", time.Now().UTC())
	if err := s.app.Save(record); err != nil {
		return event.InternalServerError("Não foi possível rotacionar o refresh token.", err)
	}
	return s.issueTokens(event, record.GetString("owner"), clientID, strings.Fields(record.GetString("scopes")), resource)
}

func (s *oauthService) issueTokens(event *core.RequestEvent, ownerID, clientID string, scopes []string, resource string) error {
	accessToken, err := randomToken()
	if err != nil {
		return event.InternalServerError("Não foi possível emitir o access token.", err)
	}
	refreshToken, err := randomToken()
	if err != nil {
		return event.InternalServerError("Não foi possível emitir o refresh token.", err)
	}
	accessCollection, err := s.app.FindCollectionByNameOrId(pb_migrations.OAuthAccessCollection)
	if err != nil {
		return event.InternalServerError("Tokens OAuth indisponíveis.", err)
	}
	access := core.NewRecord(accessCollection)
	access.Set("owner", ownerID)
	access.Set("client_id", clientID)
	access.Set("token_hash", tokenHash(accessToken))
	access.Set("scopes", strings.Join(scopes, " "))
	access.Set("resource", resource)
	access.Set("expires_at", time.Now().UTC().Add(accessTokenTTL))
	if err := s.app.Save(access); err != nil {
		return event.InternalServerError("Não foi possível salvar o access token.", err)
	}
	refreshCollection, err := s.app.FindCollectionByNameOrId(pb_migrations.OAuthRefreshCollection)
	if err != nil {
		_ = s.app.Delete(access)
		return event.InternalServerError("Refresh tokens OAuth indisponíveis.", err)
	}
	refresh := core.NewRecord(refreshCollection)
	refresh.Set("owner", ownerID)
	refresh.Set("client_id", clientID)
	refresh.Set("token_hash", tokenHash(refreshToken))
	refresh.Set("scopes", strings.Join(scopes, " "))
	refresh.Set("resource", resource)
	refresh.Set("expires_at", time.Now().UTC().Add(refreshTokenTTL))
	if err := s.app.Save(refresh); err != nil {
		_ = s.app.Delete(access)
		return event.InternalServerError("Não foi possível salvar o refresh token.", err)
	}
	writeAudit(s.app, ownerID, "", "oauth.token", "success", map[string]any{"client": clientID})
	return event.JSON(http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    int(accessTokenTTL.Seconds()),
		"refresh_token": refreshToken,
		"scope":         strings.Join(scopes, " "),
	})
}

func (s *oauthService) revokeToken(event *core.RequestEvent) error {
	if err := event.Request.ParseForm(); err != nil {
		return oauthJSONError(event, http.StatusBadRequest, "invalid_request", "Formulário inválido.")
	}
	token := strings.TrimSpace(event.Request.PostFormValue("token"))
	clientID := strings.TrimSpace(event.Request.PostFormValue("client_id"))
	if token == "" {
		return oauthJSONError(event, http.StatusBadRequest, "invalid_request", "token é obrigatório.")
	}
	hash := tokenHash(token)
	for _, collection := range []string{pb_migrations.OAuthAccessCollection, pb_migrations.OAuthRefreshCollection} {
		record, err := s.app.FindFirstRecordByFilter(collection, "token_hash = {:hash}", dbx.Params{"hash": hash})
		if err != nil || (clientID != "" && record.GetString("client_id") != clientID) {
			continue
		}
		record.Set("revoked_at", time.Now().UTC())
		_ = s.app.Save(record)
		writeAudit(s.app, record.GetString("owner"), "", "oauth.revoke", "success", map[string]any{"client": record.GetString("client_id")})
		break
	}
	event.Response.Header().Set("Cache-Control", "no-store")
	return event.NoContent(http.StatusOK)
}

func (s *oauthService) verifyToken(token string) (*auth.TokenInfo, error) {
	record, err := s.app.FindFirstRecordByFilter(pb_migrations.OAuthAccessCollection, "token_hash = {:hash}", dbx.Params{"hash": tokenHash(token)})
	if err != nil || !record.GetDateTime("revoked_at").IsZero() {
		return nil, auth.ErrInvalidToken
	}
	expires := record.GetDateTime("expires_at").Time()
	if !expires.After(time.Now().UTC()) || record.GetString("resource") != s.resource {
		return nil, auth.ErrInvalidToken
	}
	ownerID := record.GetString("owner")
	return &auth.TokenInfo{
		Scopes:     strings.Fields(record.GetString("scopes")),
		Expiration: expires,
		UserID:     ownerID,
		Extra:      map[string]any{"owner_id": ownerID, "client_id": record.GetString("client_id")},
	}, nil
}

func (s *oauthService) findClient(clientID string) (*core.Record, error) {
	return s.app.FindFirstRecordByFilter(pb_migrations.OAuthClientsCollection, "client_id = {:client}", dbx.Params{"client": clientID})
}

func authorizationFromValues(values url.Values) authorizationRequest {
	resource := strings.TrimSpace(values.Get("resource"))
	return authorizationRequest{
		ClientID:            strings.TrimSpace(values.Get("client_id")),
		RedirectURI:         strings.TrimSpace(values.Get("redirect_uri")),
		ResponseType:        strings.TrimSpace(values.Get("response_type")),
		Scope:               strings.TrimSpace(values.Get("scope")),
		State:               values.Get("state"),
		CodeChallenge:       strings.TrimSpace(values.Get("code_challenge")),
		CodeChallengeMethod: strings.TrimSpace(values.Get("code_challenge_method")),
		Resource:            resource,
	}
}

func normalizeScopes(raw string) ([]string, error) {
	requested := strings.Fields(raw)
	if len(requested) == 0 {
		return append([]string(nil), oauthresource.AllScopes...), nil
	}
	allowed := make(map[string]int, len(oauthresource.AllScopes))
	for index, scope := range oauthresource.AllScopes {
		allowed[scope] = index
	}
	unique := map[string]struct{}{}
	for _, scope := range requested {
		if _, ok := allowed[scope]; !ok {
			return nil, fmt.Errorf("escopo OAuth não suportado: %s", scope)
		}
		unique[scope] = struct{}{}
	}
	if _, ok := unique[oauthresource.ScopeRead]; !ok {
		return nil, fmt.Errorf("o escopo browser:read é obrigatório")
	}
	result := make([]string, 0, len(unique))
	for scope := range unique {
		result = append(result, scope)
	}
	sort.Slice(result, func(i, j int) bool { return allowed[result[i]] < allowed[result[j]] })
	return result, nil
}

func clientAllowsRedirect(client *core.Record, redirect string) bool {
	for _, value := range client.GetStringSlice("redirect_uris") {
		if value == redirect {
			return true
		}
	}
	return false
}

func validatedPublicMCPURL(raw string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Host == "" {
		return "", "", fmt.Errorf("NAVEGO_PUBLIC_MCP_URL inválida")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return "", "", fmt.Errorf("NAVEGO_PUBLIC_MCP_URL deve usar HTTPS, exceto em localhost")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path != "/mcp" {
		return "", "", fmt.Errorf("NAVEGO_PUBLIC_MCP_URL deve terminar em /mcp")
	}
	return parsed.String(), parsed.Scheme + "://" + parsed.Host, nil
}

func validatedRedirectURI(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.User != nil || parsed.Fragment != "" || parsed.Host == "" {
		return "", fmt.Errorf("redirect_uri inválida")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return "", fmt.Errorf("redirect_uri deve usar HTTPS, exceto em localhost")
	}
	return parsed.String(), nil
}

func oauthRedirectOrigin(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func oauthJSONError(event *core.RequestEvent, status int, code, description string) error {
	event.Response.Header().Set("Cache-Control", "no-store")
	return event.JSON(status, map[string]string{"error": code, "error_description": description})
}

func redirectOAuthError(event *core.RequestEvent, rawRedirect, state, issuer, code, description string) error {
	redirect, err := url.Parse(rawRedirect)
	if err != nil {
		return renderOAuthError(event, description)
	}
	query := redirect.Query()
	query.Set("error", code)
	query.Set("error_description", description)
	if state != "" {
		query.Set("state", state)
	}
	query.Set("iss", issuer)
	redirect.RawQuery = query.Encode()
	return event.Redirect(http.StatusFound, redirect.String())
}

type authorizationPageData struct {
	Request        authorizationRequest
	ClientName     string
	Scopes         []string
	Email          string
	Error          string
	FormAction     string
	Issuer         string
	RedirectOrigin string
}

var authorizationPage = template.Must(template.New("authorize").Parse(`<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Autorizar Navego</title><style>
*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:#070707;color:#f7f7f7;font:15px/1.5 Inter,ui-sans-serif,system-ui;padding:24px}.card{width:min(100%,470px);background:#111;border:1px solid #2f2f2f;border-radius:24px;padding:28px;box-shadow:0 30px 90px #000}.brand{display:flex;align-items:center;gap:11px;font-weight:650;font-size:18px}.navy{width:34px;height:27px;border-radius:8px;background:#f6f6f6;position:relative;box-shadow:inset 0 0 0 1px #ccc}.navy:before,.navy:after{content:"";position:absolute;width:5px;height:9px;border-radius:99px;background:#111;top:9px}.navy:before{left:10px}.navy:after{right:10px}.eyebrow{color:#03c872;font:11px ui-monospace,monospace;letter-spacing:.14em;text-transform:uppercase;margin-top:28px}h1{font-size:27px;line-height:1.15;letter-spacing:-.035em;margin:8px 0 10px}p{color:#999;margin:0 0 20px}.permissions{background:#1b1b1b;border:1px solid #2f2f2f;border-radius:14px;padding:12px 14px;margin:18px 0}.permissions div{padding:5px 0;color:#ccc}.permissions div:before{content:"✓";color:#03c872;margin-right:9px}label{display:block;color:#bbb;font-size:12px;margin:13px 0 6px}input{width:100%;border:1px solid #313131;background:#0c0c0c;color:#fff;border-radius:11px;padding:12px 13px;font:inherit;outline:none}input:focus{border-color:#03c872;box-shadow:0 0 0 3px #03c87222}.error{color:#ff8585;background:#401a1a;border-radius:10px;padding:10px 12px;margin:12px 0}.actions{display:flex;gap:10px;margin-top:20px}.actions button{flex:1;border-radius:999px;padding:12px;border:1px solid #333;background:#202020;color:#fff;font-weight:650;cursor:pointer}.actions .approve{background:#f5f5f5;color:#070707;border-color:#f5f5f5}.hint{font-size:11px;color:#777;margin-top:16px;text-align:center}
</style></head><body><main class="card"><div class="brand"><span class="navy"></span>Navego</div><div class="eyebrow">Conexão MCP segura</div><h1>Autorizar {{.ClientName}}</h1><p>Essa conexão poderá controlar somente os Chromiums da sua conta. Você ainda confirma ações externas sensíveis.</p><div class="permissions">{{range .Scopes}}<div>{{.}}</div>{{end}}</div>{{if .Error}}<div class="error">{{.Error}}</div>{{end}}<form method="post" action="{{.FormAction}}">
<input type="hidden" name="client_id" value="{{.Request.ClientID}}"><input type="hidden" name="redirect_uri" value="{{.Request.RedirectURI}}"><input type="hidden" name="response_type" value="{{.Request.ResponseType}}"><input type="hidden" name="scope" value="{{.Request.Scope}}"><input type="hidden" name="state" value="{{.Request.State}}"><input type="hidden" name="code_challenge" value="{{.Request.CodeChallenge}}"><input type="hidden" name="code_challenge_method" value="{{.Request.CodeChallengeMethod}}"><input type="hidden" name="resource" value="{{.Request.Resource}}">
<label for="email">E-mail do Navego</label><input id="email" type="email" name="email" autocomplete="username" required value="{{.Email}}"><label for="password">Senha</label><input id="password" type="password" name="password" autocomplete="current-password" required><div class="actions"><button name="decision" value="deny" formnovalidate>Cancelar</button><button class="approve" name="decision" value="approve">Entrar e autorizar</button></div></form><div class="hint">Sua senha é validada no PocketBase e nunca é enviada ao ChatGPT.</div></main></body></html>`))

func renderAuthorizationPage(event *core.RequestEvent, data authorizationPageData) error {
	event.Response.Header().Set("Cache-Control", "no-store")
	formActionSources := []string{"'self'", data.Issuer}
	if data.RedirectOrigin != "" && data.RedirectOrigin != data.Issuer {
		formActionSources = append(formActionSources, data.RedirectOrigin)
	}
	event.Response.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action "+strings.Join(formActionSources, " ")+"; frame-ancestors 'none'; base-uri 'none'")
	event.Response.Header().Set("X-Frame-Options", "DENY")
	event.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	event.Response.WriteHeader(http.StatusOK)
	return authorizationPage.Execute(event.Response, data)
}

func renderOAuthError(event *core.RequestEvent, message string) error {
	event.Response.Header().Set("Cache-Control", "no-store")
	event.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	event.Response.WriteHeader(http.StatusBadRequest)
	return template.Must(template.New("error").Parse(`<!doctype html><html lang="pt-BR"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>OAuth inválido</title><body style="background:#070707;color:#eee;font:16px system-ui;display:grid;place-items:center;min-height:100vh"><main style="max-width:520px;padding:30px;border:1px solid #333;border-radius:20px;background:#111"><h1>Não foi possível conectar</h1><p style="color:#aaa">{{.}}</p></main></body></html>`)).Execute(event.Response, message)
}

// Compile-time assertion helper used by the MCP handler.
func (s *oauthService) verifier() auth.TokenVerifier {
	return func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		return s.verifyToken(token)
	}
}

// Keep encoding/json referenced here because PocketBase JSON fields can be
// returned as JSONRaw in older databases; this also gives clientAllowsRedirect
// a safe fallback for migrated records.
func decodeStringList(value any) []string {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var result []string
	_ = json.Unmarshal(raw, &result)
	return result
}
