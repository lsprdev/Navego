package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lsprdev/Navego/internal/agent"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	controlURL := envOrDefault("NAVEGO_CONTROL_URL", "http://navego-control:8090")
	agentID := envOrDefault("NAVEGO_AGENT_ID", "primary")
	agentToken := strings.TrimSpace(os.Getenv("NAVEGO_AGENT_TOKEN"))

	control, err := agent.NewHTTPControlClient(controlURL, agentToken, &http.Client{Timeout: 15 * time.Second})
	if err != nil {
		logger.Error("invalid control plane configuration", "error", err)
		os.Exit(1)
	}
	engine, err := agent.NewDockerEngine(agent.DockerConfig{
		BrowserImage: envOrDefault("NAVEGO_BROWSER_IMAGE", "navego-browser:local"),
		WorkerImage:  envOrDefault("NAVEGO_WORKER_IMAGE", "navego-runtime:local"),
		Network:      envOrDefault("NAVEGO_DOCKER_NETWORK", "navego-runtime"),
		Timezone:     envOrDefault("TZ", "America/Sao_Paulo"),
		PUID:         envOrDefault("PUID", "1000"),
		PGID:         envOrDefault("PGID", "1000"),
		WorkerAPIKey: strings.TrimSpace(os.Getenv("NAVEGO_WORKER_API_KEY")),
	})
	if err != nil {
		logger.Error("cannot initialize Docker engine", "error", err)
		os.Exit(1)
	}
	defer engine.Close()

	runner, err := agent.NewRunner(control, engine, agentID, pollInterval(), logger)
	if err != nil {
		logger.Error("cannot initialize agent", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Info("Navego Agent started", "agent_id", agentID, "control_url", controlURL)
	if err := runner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("Navego Agent stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func pollInterval() time.Duration {
	value := strings.TrimSpace(os.Getenv("NAVEGO_AGENT_POLL_INTERVAL"))
	if value == "" {
		return 2 * time.Second
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < 250*time.Millisecond {
		return 2 * time.Second
	}
	return duration
}
