package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/lsprdev/Navego/internal/browser"
	"github.com/lsprdev/Navego/internal/oauthresource"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

const maxMCPRequestBytes = 1 << 20

type Options struct {
	MCPServer          *mcp.Server
	Browser            browser.Controller
	APIKey             string
	OAuth              *OAuthOptions
	SessionIdleTimeout time.Duration
	Logger             *slog.Logger
}

type OAuthOptions struct {
	PublicURL           string
	AuthorizationServer string
	TokenVerifier       auth.TokenVerifier
}

func New(opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var mcpHandler http.Handler = mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return opts.MCPServer },
		&mcp.StreamableHTTPOptions{
			Stateless:                    false,
			JSONResponse:                 true,
			Logger:                       logger,
			SessionTimeout:               opts.SessionIdleTimeout,
			MaxRequestBodyBytes:          maxMCPRequestBytes,
			PropagateRequestCancellation: true,
		},
	)
	if opts.OAuth != nil {
		mcpHandler = advertiseToolSecuritySchemes(mcpHandler, logger)
	}

	protectedMCP := http.NewCrossOriginProtection().Handler(mcpHandler)
	if opts.OAuth != nil {
		metadataURL, _ := oauthresource.ProtectedResourceMetadataLocations(opts.OAuth.PublicURL)
		protectedMCP = auth.RequireBearerToken(opts.OAuth.TokenVerifier, &auth.RequireBearerTokenOptions{
			ResourceMetadataURL: metadataURL,
			Scopes:              []string{oauthresource.ScopeRead},
		})(protectedMCP)
	} else if opts.APIKey != "" {
		protectedMCP = bearerAuth(opts.APIKey, protectedMCP)
	}

	mux := http.NewServeMux()
	if opts.OAuth != nil {
		_, metadataPaths := oauthresource.ProtectedResourceMetadataLocations(opts.OAuth.PublicURL)
		metadataHandler := auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
			Resource:               opts.OAuth.PublicURL,
			AuthorizationServers:   []string{opts.OAuth.AuthorizationServer},
			ScopesSupported:        oauthresource.AllScopes,
			BearerMethodsSupported: []string{"header"},
			ResourceName:           "Navego",
		})
		for _, path := range metadataPaths {
			mux.Handle("GET "+path, metadataHandler)
			mux.Handle("OPTIONS "+path, metadataHandler)
		}
	}
	mux.Handle("GET /mcp", protectedMCP)
	mux.Handle("POST /mcp", protectedMCP)
	mux.Handle("DELETE /mcp", protectedMCP)
	mux.HandleFunc("GET /healthz", healthHandler(opts.Browser))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"name":     "navego",
			"version":  "0.1.0",
			"mcp_path": "/mcp",
		})
	})

	return securityHeaders(mux)
}

// advertiseToolSecuritySchemes copies the auth schemes already present in each
// tool's _meta to the top-level securitySchemes extension expected by ChatGPT.
// The MCP Go SDK v1.7.0 does not yet expose that extension on mcp.Tool, so this
// compatibility layer touches only tools/list JSON responses.
func advertiseToolSecuritySchemes(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}
		originalBody := r.Body
		body, err := io.ReadAll(io.LimitReader(originalBody, maxMCPRequestBytes+1))
		_ = originalBody.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		if len(body) > maxMCPRequestBytes || !isToolsListRequest(body) {
			next.ServeHTTP(w, r)
			return
		}

		buffered := newBufferedResponseWriter()
		next.ServeHTTP(buffered, r)
		responseBody := buffered.body.Bytes()
		if buffered.status >= 200 && buffered.status < 300 {
			rewritten, rewriteErr := addTopLevelSecuritySchemes(responseBody)
			if rewriteErr != nil {
				logger.Warn("could not advertise top-level tool security schemes", "error", rewriteErr)
			} else {
				responseBody = rewritten
			}
		}

		copyHeaders(w.Header(), buffered.header)
		w.Header().Del("Content-Length")
		w.WriteHeader(buffered.status)
		_, _ = w.Write(responseBody)
	})
}

func isToolsListRequest(body []byte) bool {
	var request struct {
		Method string `json:"method"`
	}
	return json.Unmarshal(body, &request) == nil && request.Method == "tools/list"
}

func addTopLevelSecuritySchemes(body []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var envelope map[string]any
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode tools/list response: %w", err)
	}
	result, ok := envelope["result"].(map[string]any)
	if !ok {
		return body, nil
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		return body, nil
	}
	for _, value := range tools {
		tool, ok := value.(map[string]any)
		if !ok {
			continue
		}
		meta, ok := tool["_meta"].(map[string]any)
		if !ok {
			continue
		}
		if schemes, exists := meta["securitySchemes"]; exists {
			tool["securitySchemes"] = schemes
		}
	}
	rewritten, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode tools/list response: %w", err)
	}
	return rewritten, nil
}

type bufferedResponseWriter struct {
	header      http.Header
	body        bytes.Buffer
	status      int
	wroteHeader bool
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{header: make(http.Header), status: http.StatusOK}
}

func (w *bufferedResponseWriter) Header() http.Header { return w.header }

func (w *bufferedResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
}

func (w *bufferedResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(data)
}

func (w *bufferedResponseWriter) Flush() {}

func copyHeaders(destination, source http.Header) {
	for name, values := range source {
		destination.Del(name)
		for _, value := range values {
			destination.Add(name, value)
		}
	}
}

func healthHandler(controller browser.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		status, err := controller.Status(ctx)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "unhealthy",
				"error":  err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"browser": status,
		})
	}
}

func bearerAuth(apiKey string, next http.Handler) http.Handler {
	want := sha256.Sum256([]byte(apiKey))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		got := sha256.Sum256([]byte(token))
		if !ok || !strings.EqualFold(scheme, "Bearer") || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="navego"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
