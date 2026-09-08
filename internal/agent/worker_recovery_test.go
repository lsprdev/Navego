package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"
)

func TestWorkerPredatesBrowser(t *testing.T) {
	state := func(at string) *container.State { return &container.State{Running: true, StartedAt: at} }
	for _, tc := range []struct {
		name            string
		worker, browser *container.State
		want            bool
	}{
		{"host reboot", state("2026-09-08T00:00:00Z"), state("2026-09-08T00:00:10Z"), true},
		{"already reattached", state("2026-09-08T00:00:20Z"), state("2026-09-08T00:00:10Z"), false},
		{"same time", state("2026-09-08T00:00:10Z"), state("2026-09-08T00:00:10Z"), false},
		{"invalid timestamp", state(""), state("2026-09-08T00:00:10Z"), false},
		{"missing state", nil, state("2026-09-08T00:00:10Z"), false},
		{"stopped worker", &container.State{}, state("2026-09-08T00:00:10Z"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := workerPredatesBrowser(tc.worker, tc.browser); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// Exercise actual Docker client calls through a fake transport: these tests
// never connect to the user's Docker daemon or restart a real container.
type recoveryDocker struct {
	workerStart  string
	browserStart string
	labels       map[string]string
	restarts     int
	restartFails bool
	workerImage  string
	desiredImage string
	imageMissing bool
	replacements int
	creates      int
}

func (d *recoveryDocker) engine(t *testing.T) *DockerEngine {
	t.Helper()
	client, err := dockerclient.New(dockerclient.WithHost("http://docker.test"), dockerclient.WithAPIVersion("1.55"), dockerclient.WithHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			code, body := 200, ""
			switch {
			case req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/images/"):
				body = fmt.Sprintf(`{"Id":%q}`, d.desiredImage)
				if d.imageMissing {
					code, body = 404, `{"message":"image missing"}`
				}
			case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/json"):
				labels, started, id := d.labels, d.workerStart, "worker-id"
				if strings.Contains(req.URL.Path, "navego-browser-browser1") {
					labels, started, id = runtimeLabels(Browser{ID: "browser1"}, "browser"), d.browserStart, "browser-id"
				}
				data, _ := json.Marshal(map[string]any{
					"Id": id, "Image": d.workerImage, "State": map[string]any{"Running": true, "StartedAt": started},
					"Config":     map[string]any{"Labels": labels},
					"HostConfig": map[string]any{"NetworkMode": "container:browser-id"},
				})
				body = string(data)
			case req.Method == http.MethodPost && req.URL.Path == "/v1.55/containers/worker-id/stop":
				code = http.StatusNoContent
			case req.Method == http.MethodDelete && req.URL.Path == "/v1.55/containers/worker-id":
				d.replacements++
				code = http.StatusNoContent
			case req.Method == http.MethodPost && req.URL.Path == "/v1.55/containers/create":
				var config struct {
					Image      string
					HostConfig struct{ NetworkMode string }
				}
				if err := json.NewDecoder(req.Body).Decode(&config); err != nil {
					t.Fatal(err)
				}
				if config.Image != d.desiredImage || config.HostConfig.NetworkMode != "container:browser-id" {
					t.Fatalf("incorrect replacement config: %+v", config)
				}
				d.creates++
				d.workerImage = config.Image
				code, body = http.StatusCreated, `{"Id":"worker-id"}`
			case req.Method == http.MethodPost && req.URL.Path == "/v1.55/containers/worker-id/start":
				code = http.StatusNoContent
			case req.Method == http.MethodPost && req.URL.Path == "/v1.55/containers/worker-id/restart":
				d.restarts++
				code = http.StatusNoContent
				if d.restartFails {
					code, body = 500, `{"message":"restart failed"}`
				} else {
					d.workerStart = "2026-09-08T01:00:00Z"
				}
			default:
				t.Fatalf("unexpected Docker operation: %s %s", req.Method, req.URL.Path)
			}
			return &http.Response{StatusCode: code, Status: fmt.Sprintf("%d", code), Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		}),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return &DockerEngine{client: client, cfg: DockerConfig{WorkerImage: "navego-runtime:production"}}
}

func newRecoveryDocker() *recoveryDocker {
	return &recoveryDocker{
		workerStart: "2026-09-08T00:00:20Z", browserStart: "2026-09-08T00:00:10Z",
		labels:      runtimeLabels(Browser{ID: "browser1"}, "worker"),
		workerImage: "sha256:current", desiredImage: "sha256:current",
	}
}

func TestWorkerRecoveryPersistentFailuresAndHealthyReset(t *testing.T) {
	d := newRecoveryDocker()
	e := d.engine(t)
	b := Browser{ID: "browser1"}
	names, _ := runtimeNames(b.ID)
	now := time.Date(2026, 9, 8, 0, 2, 0, 0, time.UTC)
	probe := func(offset time.Duration, err error) {
		t.Helper()
		if err := e.recoverWorker(context.Background(), b, names, err, now.Add(offset)); err != nil {
			t.Fatal(err)
		}
	}
	unhealthy := errors.New("health timeout")
	probe(0, unhealthy)
	probe(time.Minute, unhealthy)
	if d.restarts != 0 {
		t.Fatal("restarted during a normal navigation window")
	}
	probe(70*time.Second, nil)
	probe(80*time.Second, unhealthy)
	probe(90*time.Second, unhealthy)
	probe(199*time.Second, unhealthy)
	if d.restarts != 0 {
		t.Fatal("healthy response did not reset the failure window")
	}
	probe(200*time.Second, unhealthy)
	if d.restarts != 1 {
		t.Fatalf("wanted one worker restart, got %d", d.restarts)
	}
	probe(201*time.Second, unhealthy)
	probe(202*time.Second, unhealthy)
	if d.restarts != 1 {
		t.Fatal("restart storm during recovery")
	}
	probe(203*time.Second, nil)
	if len(e.recovery) != 0 {
		t.Fatal("successful recovery left failure state")
	}
}

func TestWorkerRecoveryCooldownAfterDockerFailure(t *testing.T) {
	d := newRecoveryDocker()
	d.restartFails = true
	e := d.engine(t)
	names, _ := runtimeNames("browser1")
	now := time.Now()
	for _, offset := range []time.Duration{0, time.Minute, 2 * time.Minute, 121 * time.Second, 122 * time.Second} {
		err := e.recoverWorker(context.Background(), Browser{ID: "browser1"}, names, errors.New("unhealthy"), now.Add(offset))
		if (offset == 2*time.Minute) != (err != nil) {
			t.Fatalf("unexpected recovery error at %s: %v", offset, err)
		}
	}
	if d.restarts != 1 {
		t.Fatalf("failed restart was retried without cooldown: %d", d.restarts)
	}
}

func TestWorkerRecoveryRejectsUnownedContainerAndCancellation(t *testing.T) {
	d := newRecoveryDocker()
	d.labels = runtimeLabels(Browser{ID: "another-browser"}, "worker")
	e := d.engine(t)
	names, _ := runtimeNames("browser1")
	if err := e.recoverWorker(context.Background(), Browser{ID: "browser1"}, names, errors.New("unhealthy"), time.Now()); err == nil {
		t.Fatal("accepted an unowned container")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.recoverWorker(ctx, Browser{ID: "browser1"}, names, errors.New("unhealthy"), time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if d.restarts != 0 {
		t.Fatal("restarted despite failed ownership or canceled agent")
	}
}

func TestEnsureWorkerReattachesOnceAfterExternalBrowserRestart(t *testing.T) {
	d := newRecoveryDocker()
	d.workerStart = "2026-09-08T00:00:00Z"
	e := d.engine(t)
	b := Browser{ID: "browser1"}
	names, _ := runtimeNames(b.ID)
	for i := 0; i < 3; i++ {
		id, err := e.ensureWorkerContainer(context.Background(), b, names, "browser-id", false)
		if err != nil {
			t.Fatal(err)
		}
		if id != "worker-id" {
			t.Fatalf("unexpected worker: %s", id)
		}
	}
	if d.restarts != 1 {
		t.Fatalf("wanted one reattachment after host reboot, got %d", d.restarts)
	}
}

func TestWorkerRecoveryResetsOnContainerRestartOrReplacement(t *testing.T) {
	now := time.Now()
	for _, replacement := range []bool{false, true} {
		r := workerRecovery{}
		if r.failed("worker1", "boot1", now) || r.failed("worker1", "boot1", now.Add(time.Minute)) {
			t.Fatal("premature recovery")
		}
		id := "worker1"
		if replacement {
			id = "worker2"
		}
		if r.failed(id, "boot2", now.Add(3*time.Minute)) {
			t.Fatal("inherited stale failures from the previous worker process")
		}
	}
}

func TestEnsureWorkerAdoptsRebuiltImageOnce(t *testing.T) {
	d := newRecoveryDocker()
	d.desiredImage = "sha256:rebuilt"
	e := d.engine(t)
	b := Browser{ID: "browser1"}
	names, _ := runtimeNames(b.ID)
	for i := 0; i < 3; i++ {
		if _, err := e.ensureWorkerContainer(context.Background(), b, names, "browser-id", false); err != nil {
			t.Fatal(err)
		}
	}
	if d.replacements != 1 || d.creates != 1 || d.restarts != 0 {
		t.Fatalf("expected one image replacement, got deletes=%d creates=%d restarts=%d", d.replacements, d.creates, d.restarts)
	}
}

func TestEnsureWorkerPreservesContainerWhenImageUnavailable(t *testing.T) {
	d := newRecoveryDocker()
	d.imageMissing = true
	e := d.engine(t)
	b := Browser{ID: "browser1"}
	names, _ := runtimeNames(b.ID)
	if _, err := e.ensureWorkerContainer(context.Background(), b, names, "browser-id", false); err == nil {
		t.Fatal("expected missing image error")
	}
	if d.replacements != 0 || d.creates != 0 || d.restarts != 0 {
		t.Fatal("modified worker before checking image availability")
	}
}
