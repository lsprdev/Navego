package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestHTTPControlClientAuthenticatesAndDecodesCommands(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("missing agent token")
		}
		if request.URL.Query().Get("agent_id") != "agent-1" {
			t.Fatalf("missing agent ID")
		}
		var body bytes.Buffer
		_ = json.NewEncoder(&body).Encode([]Browser{{ID: "browser1", State: "queued"}})
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(body.Bytes())),
			Request:    request,
		}, nil
	})}

	client, err := NewHTTPControlClient("http://control.test", "test-token", httpClient)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := client.Commands(context.Background(), "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0].ID != "browser1" {
		t.Fatalf("unexpected commands: %#v", commands)
	}
}

func TestHTTPControlClientRejectsMissingConfiguration(t *testing.T) {
	if _, err := NewHTTPControlClient("not-a-url", "token", nil); err == nil {
		t.Fatal("expected invalid URL error")
	}
	if _, err := NewHTTPControlClient("http://control:8090", "", nil); err == nil {
		t.Fatal("expected missing token error")
	}
}
