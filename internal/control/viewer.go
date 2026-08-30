package control

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

const viewerCookieName = "navego_viewer"

type viewerGrant struct {
	endpoint  string
	expiresAt time.Time
}

type viewerStore struct {
	mu       sync.Mutex
	tickets  map[string]viewerGrant
	sessions map[string]viewerGrant
	now      func() time.Time
}

func newViewerStore() *viewerStore {
	return &viewerStore{
		tickets:  make(map[string]viewerGrant),
		sessions: make(map[string]viewerGrant),
		now:      time.Now,
	}
}

func (s *viewerStore) mint(endpoint string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	s.tickets[token] = viewerGrant{endpoint: endpoint, expiresAt: s.now().Add(time.Minute)}
	return token, nil
}

func (s *viewerStore) consume(ticket string) (string, viewerGrant, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	grant, ok := s.tickets[ticket]
	if !ok {
		return "", viewerGrant{}, false
	}
	delete(s.tickets, ticket)
	session, err := randomToken()
	if err != nil {
		return "", viewerGrant{}, false
	}
	grant.expiresAt = s.now().Add(20 * time.Minute)
	s.sessions[session] = grant
	return session, grant, true
}

func (s *viewerStore) resolve(session string) (viewerGrant, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	grant, ok := s.sessions[session]
	return grant, ok
}

func (s *viewerStore) cleanupLocked() {
	now := s.now()
	for token, grant := range s.tickets {
		if !grant.expiresAt.After(now) {
			delete(s.tickets, token)
		}
	}
	for token, grant := range s.sessions {
		if !grant.expiresAt.After(now) {
			delete(s.sessions, token)
		}
	}
}

func randomToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func mintViewerTicket(store *viewerStore, publicURL string) func(*core.RequestEvent) error {
	return func(event *core.RequestEvent) error {
		record, apiErr := ownedBrowser(event)
		if apiErr != nil {
			return apiErr
		}
		if record.GetString("state") != "running" {
			return apis.NewApiError(http.StatusConflict, "O navegador ainda não está disponível.", nil)
		}
		endpoint, err := validatedViewerEndpoint(record.GetString("viewer_endpoint"))
		if err != nil {
			return event.InternalServerError("Endpoint interno do viewer inválido.", err)
		}
		ticket, err := store.mint(endpoint)
		if err != nil {
			return event.InternalServerError("Não foi possível criar o acesso temporário.", err)
		}
		base, err := validatedPublicViewerURL(publicURL)
		if err != nil {
			return event.InternalServerError("URL pública do viewer inválida.", err)
		}
		return event.JSON(http.StatusCreated, map[string]string{
			"url":         base + "/viewer/session/" + ticket,
			"session_url": base + "/viewer/",
		})
	}
}

func beginViewerSession(store *viewerStore, secureCookie bool, dashboardOrigins string) func(*core.RequestEvent) error {
	return func(event *core.RequestEvent) error {
		if err := allowViewerEmbedding(event.Response.Header(), dashboardOrigins); err != nil {
			return event.InternalServerError("Origem pública do dashboard inválida.", err)
		}
		ticket := strings.TrimSpace(event.Request.PathValue("ticket"))
		session, _, ok := store.consume(ticket)
		if !ok {
			return event.UnauthorizedError("Este acesso ao navegador expirou ou já foi usado.", nil)
		}
		event.SetCookie(&http.Cookie{
			Name:     viewerCookieName,
			Value:    session,
			Path:     "/",
			MaxAge:   int((20 * time.Minute).Seconds()),
			HttpOnly: true,
			Secure:   secureCookie,
			SameSite: http.SameSiteStrictMode,
		})
		event.Response.Header().Set("Cache-Control", "private, no-store")
		event.Response.Header().Set("Referrer-Policy", "no-referrer")
		return event.Redirect(http.StatusSeeOther, "/viewer/")
	}
}

func proxyViewer(store *viewerStore, stripViewerPrefix bool, dashboardOrigins string) func(*core.RequestEvent) error {
	return func(event *core.RequestEvent) error {
		if err := allowViewerEmbedding(event.Response.Header(), dashboardOrigins); err != nil {
			return event.InternalServerError("Origem pública do dashboard inválida.", err)
		}
		cookie, err := event.Request.Cookie(viewerCookieName)
		if err != nil {
			return event.UnauthorizedError("Abra o navegador novamente pelo dashboard.", nil)
		}
		grant, ok := store.resolve(cookie.Value)
		if !ok {
			return event.UnauthorizedError("A sessão do navegador expirou.", nil)
		}
		target, err := url.Parse(grant.endpoint)
		if err != nil {
			return event.InternalServerError("Endpoint interno do viewer inválido.", err)
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		originalDirector := proxy.Director
		proxy.Director = func(request *http.Request) {
			originalDirector(request)
			request.Host = target.Host
			if stripViewerPrefix {
				request.URL.Path = strings.TrimPrefix(request.URL.Path, "/viewer")
				if request.URL.Path == "" {
					request.URL.Path = "/"
				}
			}
		}
		proxy.ModifyResponse = func(response *http.Response) error {
			response.Header.Del("X-Frame-Options")
			// The response writer already carries Navego's explicit policy. Drop any
			// upstream policy so ReverseProxy does not append a duplicate/conflicting
			// frame-ancestors directive.
			response.Header.Del("Content-Security-Policy")
			response.Header.Set("Cache-Control", "private, no-store")
			response.Header.Set("Referrer-Policy", "no-referrer")
			return nil
		}
		proxy.ErrorHandler = func(response http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(response, "Navegador temporariamente indisponível.", http.StatusBadGateway)
		}
		proxy.ServeHTTP(event.Response, event.Request)
		return nil
	}
}

func allowViewerEmbedding(header http.Header, dashboardOrigins string) error {
	if _, err := validatedPublicOrigins(dashboardOrigins); err != nil {
		return err
	}
	// PocketBase applies SAMEORIGIN globally. The local dashboard and viewer use
	// different ports, so the viewer needs an explicit frame-ancestors policy.
	header.Del("X-Frame-Options")
	header.Set("Content-Security-Policy", frameAncestorsPolicy(dashboardOrigins))
	return nil
}

func frameAncestorsPolicy(dashboardOrigins string) string {
	origins, _ := validatedPublicOrigins(dashboardOrigins)
	return "frame-ancestors 'self' " + strings.Join(origins, " ")
}

func validatedPublicOrigins(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		origin, err := validatedPublicOrigin(part)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	if len(origins) == 0 {
		return nil, fmt.Errorf("at least one public origin is required")
	}
	if len(origins) > 10 {
		return nil, fmt.Errorf("too many public origins")
	}
	return origins, nil
}

func validatedPublicOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("malformed public origin")
	}
	if parsed.Path != "" {
		return "", fmt.Errorf("public origin must not contain a path")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func validatedViewerEndpoint(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("malformed viewer endpoint")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if !strings.HasPrefix(hostname, "navego-browser-") || parsed.Port() != "3000" {
		return "", fmt.Errorf("viewer endpoint is outside the runtime network")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func validatedPublicViewerURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("malformed public viewer URL")
	}
	if parsed.Path != "" {
		return "", fmt.Errorf("public viewer URL must not contain a path")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
