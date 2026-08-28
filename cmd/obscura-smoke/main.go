package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lsprdev/Navego/internal/obscura"
)

func main() {
	endpoint := envOr("OBSCURA_MCP_ENDPOINT", "http://127.0.0.1:18080/mcp")
	targetURL := envOr("OBSCURA_SMOKE_URL", "https://example.com")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := obscura.New(endpoint, &http.Client{Timeout: 70 * time.Second}, obscura.DefaultMaxPayload)
	defer client.Close()

	status, err := client.Status(ctx)
	must("status", err)
	fmt.Printf("Obscura connected: %s %s (%d allowed tools)\n", status.ServerName, status.ServerVersion, len(status.AllowedTools))

	snapshot, err := client.Open(ctx, targetURL, "load", 2_000)
	must("open", err)
	if snapshot.URL == "" || snapshot.Title == "" {
		panic("snapshot is missing URL or title")
	}
	fmt.Printf("Open: %s — %s\n", snapshot.URL, snapshot.Title)

	markdown, err := client.Markdown(ctx, 2_000)
	must("markdown", err)
	if strings.TrimSpace(markdown) == "" {
		panic("markdown is empty")
	}
	links, err := client.Links(ctx, false, 10)
	must("links", err)
	fmt.Printf("Read: %d markdown bytes, %d links\n", len(markdown), len(links))

	image, imageType, err := client.Screenshot(ctx, 1024, 768)
	must("screenshot", err)
	fmt.Printf("Screenshot: %s, %d bytes\n", imageType, len(image))

	pdf, pdfType, err := client.PDF(ctx)
	must("pdf", err)
	fmt.Printf("PDF: %s, %d bytes\n", pdfType, len(pdf))
	fmt.Println("Obscura adapter smoke passed")
}

func must(action string, err error) {
	if err != nil {
		panic(action + ": " + err.Error())
	}
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
