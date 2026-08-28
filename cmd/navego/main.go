package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lsprdev/Navego/internal/approval"
	"github.com/lsprdev/Navego/internal/browser"
	"github.com/lsprdev/Navego/internal/config"
	"github.com/lsprdev/Navego/internal/httpserver"
	"github.com/lsprdev/Navego/internal/mcpserver"
	"github.com/lsprdev/Navego/internal/oauthresource"
	"github.com/lsprdev/Navego/internal/takeover"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	controller := browser.NewManager(
		rootCtx,
		cfg.CDPEndpoint,
		cfg.ActionTimeout,
		cfg.NavigationTimeout,
		cfg.SnapshotMaxChars,
		cfg.SnapshotMaxElements,
	)
	defer func() {
		if err := controller.Close(); err != nil {
			logger.Warn("closing browser controller", "error", err)
		}
	}()

	takeoverState := takeover.New()
	approvalStore := approval.NewStore(2 * time.Minute)
	var (
		mcpOptions []mcpserver.Option
		httpOAuth  *httpserver.OAuthOptions
	)
	if cfg.OAuthEnabled {
		discoveryCtx, cancel := context.WithTimeout(rootCtx, cfg.OAuthDiscoveryTimeout)
		oauthVerifier, discovery, err := oauthresource.New(discoveryCtx, oauthresource.Config{
			Issuer:          cfg.OAuthIssuer,
			Audience:        cfg.OAuthAudience,
			AllowedSubjects: cfg.OAuthAllowedSubjects,
		}, &http.Client{Timeout: cfg.OAuthDiscoveryTimeout})
		cancel()
		if err != nil {
			logger.Error("OAuth initialization failed", "error", err)
			os.Exit(1)
		}
		metadataURL, _ := oauthresource.ProtectedResourceMetadataLocations(cfg.PublicURL)
		mcpOptions = append(mcpOptions, mcpserver.WithAuthorization(mcpserver.Authorization{
			Enabled:             true,
			ResourceMetadataURL: metadataURL,
		}))
		httpOAuth = &httpserver.OAuthOptions{
			PublicURL:           cfg.PublicURL,
			AuthorizationServer: cfg.OAuthIssuer,
			TokenVerifier:       oauthVerifier.Verify,
		}
		logger.Info(
			"OAuth resource server enabled",
			"issuer", cfg.OAuthIssuer,
			"audience", cfg.OAuthAudience,
			"cimd", discovery.ClientIDMetadataDocumentSupported,
			"dcr", discovery.RegistrationEndpoint != "",
		)
	}
	mcpGateway := mcpserver.New(controller, takeoverState, approvalStore, cfg.HumanTakeoverURL, logger, mcpOptions...)
	handler := httpserver.New(httpserver.Options{
		MCPServer:          mcpGateway.MCP,
		Browser:            controller,
		APIKey:             cfg.APIKey,
		OAuth:              httpOAuth,
		SessionIdleTimeout: cfg.SessionIdleTimeout,
		Logger:             logger,
	})

	httpServer := &http.Server{
		Addr:              cfg.Address(),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       65 * time.Second,
		WriteTimeout:      65 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    16 << 10,
	}

	go func() {
		<-rootCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("http shutdown failed", "error", err)
		}
	}()

	logger.Info("browser MCP gateway listening", "address", cfg.Address(), "mcp_path", "/mcp")
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("http server failed", "error", err)
		os.Exit(1)
	}
}
