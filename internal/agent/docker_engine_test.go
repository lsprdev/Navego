package agent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestWorkerStatusReadsBrowserTelemetry(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/healthz" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"status":"ok","browser":{"connected":true,"title":"Example","url":"https://example.com/news"}}`,
			)),
			Request: request,
		}, nil
	})}

	engine := &DockerEngine{httpClient: httpClient}
	title, pageURL, err := engine.workerStatus(context.Background(), "http://worker.test:8001")
	if err != nil {
		t.Fatal(err)
	}
	if title != "Example" || pageURL != "https://example.com/news" {
		t.Fatalf("unexpected telemetry: title=%q url=%q", title, pageURL)
	}
}

func TestRuntimeNamesAreDeterministic(t *testing.T) {
	names, err := runtimeNames("Browser_123")
	if err != nil {
		t.Fatal(err)
	}
	if names.browser != "navego-browser-browser_123" || names.worker != "navego-worker-browser_123" || names.volume != "navego-profile-browser_123" {
		t.Fatalf("unexpected names: %#v", names)
	}
}

func TestRuntimeNamesRejectUnsafeID(t *testing.T) {
	for _, value := range []string{"", "../browser", "browser.name", "browser/name"} {
		if _, err := runtimeNames(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestValidateLabelsRejectsUnmanagedResource(t *testing.T) {
	if err := validateLabels(map[string]string{
		managedLabel: "true",
		browserLabel: "other-browser",
		roleLabel:    "browser",
	}, "browser1", "browser"); err == nil {
		t.Fatal("expected ownership validation error")
	}
}

func TestBrowserContainerNeedsRestartAfterAHealthFailure(t *testing.T) {
	tests := []struct {
		name  string
		state *container.State
		want  bool
	}{
		{name: "missing state", state: nil, want: false},
		{name: "stopped", state: &container.State{Running: false, Health: &container.Health{Status: container.Unhealthy, FailingStreak: 1}}, want: false},
		{name: "starting", state: &container.State{Running: true, Health: &container.Health{Status: container.Starting, FailingStreak: 1}}, want: false},
		{name: "healthy", state: &container.State{Running: true, Health: &container.Health{Status: container.Healthy}}, want: false},
		{name: "first failed probe", state: &container.State{Running: true, Health: &container.Health{Status: container.Healthy, FailingStreak: 1}}, want: true},
		{name: "unhealthy", state: &container.State{Running: true, Health: &container.Health{Status: container.Unhealthy, FailingStreak: 3}}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := browserContainerNeedsRestart(test.state); got != test.want {
				t.Fatalf("browserContainerNeedsRestart() = %v, want %v", got, test.want)
			}
		})
	}
}
