package main

import (
	"log"
	"os"
	"strings"

	"github.com/lsprdev/Navego/internal/control"
)

func main() {
	dataDir := strings.TrimSpace(os.Getenv("NAVEGO_DATA_DIR"))
	if dataDir == "" {
		dataDir = "./pb_data"
	}

	app := control.New(control.Config{
		DataDir:                dataDir,
		AgentToken:             os.Getenv("NAVEGO_AGENT_TOKEN"),
		WorkerAPIKey:           os.Getenv("NAVEGO_WORKER_API_KEY"),
		VaultKey:               os.Getenv("NAVEGO_VAULT_KEY"),
		PublicViewerURL:        envOrDefault("NAVEGO_PUBLIC_VIEWER_URL", "http://127.0.0.1:8090"),
		PublicDashboardURL:     envOrDefault("NAVEGO_PUBLIC_DASHBOARD_URL", "http://127.0.0.1:3000"),
		PublicMCPURL:           envOrDefault("NAVEGO_PUBLIC_MCP_URL", "http://127.0.0.1:8090/mcp"),
		PublicDashboardOrigins: envOrDefault("NAVEGO_PUBLIC_DASHBOARD_ORIGINS", "http://127.0.0.1:3000,http://localhost:3000"),
	})
	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
