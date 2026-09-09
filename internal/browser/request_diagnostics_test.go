package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
)

func TestBlockedRequestLogOmitsSecrets(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	logBlockedRequest(&fetch.EventRequestPaused{
		Request:      &network.Request{URL: "https://user:password@example.com/login-ticket?token=secret#fragment"},
		ResourceType: network.ResourceTypeDocument,
	}, "test-tab", dnsPolicyError("example.com", context.DeadlineExceeded), 10*time.Second)
	for _, secret := range []string{"password", "login-ticket", "token=", "secret", "fragment", "user:"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("log leaked %q", secret)
		}
	}
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["reason"] != "dns_timeout" || entry["host"] != "example.com" || entry["duration_ms"] != float64(10000) || entry["target_id"] != "test-tab" {
		t.Fatalf("unexpected diagnostic: %s", output.String())
	}
}
