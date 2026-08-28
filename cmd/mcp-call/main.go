package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "usage: mcp-call --info | --list | TOOL ['{\"argument\":\"value\"}']")
		os.Exit(2)
	}

	arguments := map[string]any{}
	if len(os.Args) == 3 {
		if err := json.Unmarshal([]byte(os.Args[2]), &arguments); err != nil {
			fatal("invalid arguments JSON", err)
		}
	}

	endpoint := envOr("MCP_ENDPOINT", "http://127.0.0.1:8001/mcp")
	apiKey := strings.TrimSpace(os.Getenv("NAVEGO_API_KEY"))
	httpClient := &http.Client{Timeout: 70 * time.Second}
	if apiKey != "" {
		httpClient.Transport = bearerTransport{token: apiKey, base: http.DefaultTransport}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "navego-call", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: httpClient}, nil)
	if err != nil {
		fatal("connect MCP", err)
	}
	defer session.Close()
	if os.Args[1] == "--info" {
		data, err := json.MarshalIndent(session.InitializeResult(), "", "  ")
		if err != nil {
			fatal("encode server info", err)
		}
		fmt.Println(string(data))
		return
	}
	if os.Args[1] == "--list" {
		listed, err := session.ListTools(ctx, nil)
		if err != nil {
			fatal("list tools", err)
		}
		data, err := json.MarshalIndent(listed.Tools, "", "  ")
		if err != nil {
			fatal("encode tools", err)
		}
		fmt.Println(string(data))
		return
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: os.Args[1], Arguments: arguments})
	if err != nil {
		fatal("call tool", err)
	}
	for _, content := range result.Content {
		switch value := content.(type) {
		case *mcp.TextContent:
			fmt.Println(value.Text)
		case *mcp.ImageContent:
			fmt.Printf("[image %s: %d bytes]\n", value.MIMEType, len(value.Data))
			if path := strings.TrimSpace(os.Getenv("MCP_IMAGE_OUTPUT")); path != "" {
				if err := os.WriteFile(path, value.Data, 0o600); err != nil {
					fatal("write image output", err)
				}
				fmt.Printf("image saved to %s\n", path)
			}
		default:
			fmt.Printf("[%T]\n", content)
		}
	}
	if result.StructuredContent != nil {
		data, err := json.MarshalIndent(result.StructuredContent, "", "  ")
		if err != nil {
			fatal("encode structured content", err)
		}
		fmt.Println(string(data))
	}
	if result.IsError {
		os.Exit(1)
	}
}

func fatal(action string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
	os.Exit(1)
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}
