package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	dockerclient "github.com/moby/moby/client"
)

const (
	managedLabel = "dev.lspr.navego.managed"
	browserLabel = "dev.lspr.navego.browser_id"
	ownerLabel   = "dev.lspr.navego.owner_id"
	roleLabel    = "dev.lspr.navego.role"
)

var validRuntimeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type DockerConfig struct {
	BrowserImage string
	WorkerImage  string
	Network      string
	Timezone     string
	PUID         string
	PGID         string
	WorkerAPIKey string
	HealthWait   time.Duration
}

type DockerEngine struct {
	client     *dockerclient.Client
	httpClient *http.Client
	cfg        DockerConfig
}

func NewDockerEngine(cfg DockerConfig) (*DockerEngine, error) {
	if strings.TrimSpace(cfg.BrowserImage) == "" || strings.TrimSpace(cfg.WorkerImage) == "" {
		return nil, fmt.Errorf("browser and worker images are required")
	}
	if strings.TrimSpace(cfg.Network) == "" {
		return nil, fmt.Errorf("Docker network is required")
	}
	if cfg.Timezone == "" {
		cfg.Timezone = "America/Sao_Paulo"
	}
	if cfg.PUID == "" {
		cfg.PUID = "1000"
	}
	if cfg.PGID == "" {
		cfg.PGID = "1000"
	}
	if cfg.HealthWait <= 0 {
		cfg.HealthWait = 90 * time.Second
	}
	client, err := dockerclient.New(dockerclient.FromEnv, dockerclient.WithUserAgent("navego-agent/0.1"))
	if err != nil {
		return nil, fmt.Errorf("create Docker client: %w", err)
	}
	return &DockerEngine{
		client:     client,
		httpClient: &http.Client{Timeout: 3 * time.Second},
		cfg:        cfg,
	}, nil
}

func (e *DockerEngine) Close() error {
	return e.client.Close()
}

func (e *DockerEngine) EnsureRunning(ctx context.Context, browser Browser) (Runtime, error) {
	names, err := runtimeNames(browser.ID)
	if err != nil {
		return Runtime{}, err
	}
	labels := runtimeLabels(browser, "profile")
	volume, err := e.ensureVolume(ctx, names.volume, labels)
	if err != nil {
		return Runtime{}, err
	}

	browserID, browserRestarted, err := e.ensureBrowserContainer(ctx, browser, names, volume)
	if err != nil {
		return Runtime{}, err
	}
	if err := e.waitForBrowser(ctx, names.browser); err != nil {
		return Runtime{}, err
	}
	workerID, err := e.ensureWorkerContainer(ctx, browser, names, browserID, browserRestarted)
	if err != nil {
		return Runtime{}, err
	}

	runtime := Runtime{
		BrowserContainer: browserID,
		WorkerContainer:  workerID,
		ProfileVolume:    volume,
		WorkerEndpoint:   "http://" + names.browser + ":8001",
		ViewerEndpoint:   "http://" + names.browser + ":3000",
	}
	if title, pageURL, statusErr := e.workerStatus(ctx, runtime.WorkerEndpoint); statusErr == nil {
		runtime.Title = title
		runtime.URL = pageURL
	}
	return runtime, nil
}

func (e *DockerEngine) workerStatus(ctx context.Context, endpoint string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/healthz", nil)
	if err != nil {
		return "", "", fmt.Errorf("create worker health request: %w", err)
	}
	response, err := e.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("read worker health: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("worker health returned %s", response.Status)
	}
	var envelope struct {
		Browser struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"browser"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return "", "", fmt.Errorf("decode worker health: %w", err)
	}
	return strings.TrimSpace(envelope.Browser.Title), strings.TrimSpace(envelope.Browser.URL), nil
}

func (e *DockerEngine) EnsureStopped(ctx context.Context, browser Browser) error {
	names, err := runtimeNames(browser.ID)
	if err != nil {
		return err
	}
	if err := e.stopOwnedContainer(ctx, names.worker, browser.ID, "worker"); err != nil {
		return err
	}
	return e.stopOwnedContainer(ctx, names.browser, browser.ID, "browser")
}

func (e *DockerEngine) EnsureDeleted(ctx context.Context, browser Browser) error {
	names, err := runtimeNames(browser.ID)
	if err != nil {
		return err
	}
	if err := e.removeOwnedContainer(ctx, names.worker, browser.ID, "worker"); err != nil {
		return err
	}
	if err := e.removeOwnedContainer(ctx, names.browser, browser.ID, "browser"); err != nil {
		return err
	}
	return e.removeOwnedVolume(ctx, names.volume, browser.ID)
}

type generatedNames struct {
	browser string
	worker  string
	volume  string
}

func runtimeNames(browserID string) (generatedNames, error) {
	if !validRuntimeID.MatchString(browserID) {
		return generatedNames{}, fmt.Errorf("invalid browser ID")
	}
	return generatedNames{
		browser: "navego-browser-" + strings.ToLower(browserID),
		worker:  "navego-worker-" + strings.ToLower(browserID),
		volume:  "navego-profile-" + strings.ToLower(browserID),
	}, nil
}

func runtimeLabels(browser Browser, role string) map[string]string {
	return map[string]string{
		managedLabel: "true",
		browserLabel: browser.ID,
		ownerLabel:   browser.OwnerID,
		roleLabel:    role,
	}
}

func (e *DockerEngine) ensureVolume(ctx context.Context, name string, labels map[string]string) (string, error) {
	result, err := e.client.VolumeInspect(ctx, name, dockerclient.VolumeInspectOptions{})
	if err == nil {
		if err := validateLabels(result.Volume.Labels, labels[browserLabel], "profile"); err != nil {
			return "", fmt.Errorf("refuse existing volume %q: %w", name, err)
		}
		return result.Volume.Name, nil
	}
	if !cerrdefs.IsNotFound(err) {
		return "", fmt.Errorf("inspect profile volume: %w", err)
	}
	created, err := e.client.VolumeCreate(ctx, dockerclient.VolumeCreateOptions{Name: name, Labels: labels})
	if err != nil {
		return "", fmt.Errorf("create profile volume: %w", err)
	}
	return created.Volume.Name, nil
}

func (e *DockerEngine) ensureBrowserContainer(ctx context.Context, browser Browser, names generatedNames, volume string) (string, bool, error) {
	inspect, err := e.inspectOwnedContainer(ctx, names.browser, browser.ID, "browser")
	if err == nil {
		restarted := false
		if inspect.State == nil || !inspect.State.Running {
			if _, err := e.client.ContainerStart(ctx, inspect.ID, dockerclient.ContainerStartOptions{}); err != nil {
				return "", false, fmt.Errorf("start browser container: %w", err)
			}
			restarted = true
		} else if browserContainerNeedsRestart(inspect.State) {
			timeout := 10
			if _, err := e.client.ContainerRestart(ctx, inspect.ID, dockerclient.ContainerRestartOptions{Timeout: &timeout}); err != nil {
				return "", false, fmt.Errorf("restart unhealthy browser container: %w", err)
			}
			restarted = true
		}
		return inspect.ID, restarted, nil
	}
	if !cerrdefs.IsNotFound(err) {
		return "", false, err
	}

	labels := runtimeLabels(browser, "browser")
	pids := int64(512)
	created, err := e.client.ContainerCreate(ctx, dockerclient.ContainerCreateOptions{
		Name: names.browser,
		Config: &container.Config{
			Image: e.cfg.BrowserImage,
			Env: []string{
				"PUID=" + e.cfg.PUID,
				"PGID=" + e.cfg.PGID,
				"TZ=" + e.cfg.Timezone,
				"CUSTOM_USER=abc",
				"PASSWORD=",
				"TITLE=Navego · " + cleanTitle(browser.Name),
				"RESTART_APP=true",
				"PIXELFLUX_WAYLAND=false",
				"CHROME_CLI=--remote-debugging-port=9222 --remote-debugging-address=127.0.0.1",
			},
			Labels: labels,
			Healthcheck: &container.HealthConfig{
				Test:        []string{"CMD-SHELL", `curl -fsS http://127.0.0.1:9222/json/list | grep -q '"type":[[:space:]]*"page"'`},
				Interval:    5 * time.Second,
				Timeout:     5 * time.Second,
				Retries:     3,
				StartPeriod: 20 * time.Second,
			},
		},
		HostConfig: &container.HostConfig{
			Binds:         []string{volume + ":/config"},
			RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
			ShmSize:       1 << 30,
			LogConfig:     managedLogConfig(),
			Resources: container.Resources{
				Memory:    2 << 30,
				NanoCPUs:  2_000_000_000,
				PidsLimit: &pids,
			},
		},
		NetworkingConfig: &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{
			e.cfg.Network: {Aliases: []string{names.browser}},
		}},
	})
	if err != nil {
		return "", false, fmt.Errorf("create browser container: %w", err)
	}
	if _, err := e.client.ContainerStart(ctx, created.ID, dockerclient.ContainerStartOptions{}); err != nil {
		return "", false, fmt.Errorf("start browser container: %w", err)
	}
	return created.ID, false, nil
}

func browserContainerNeedsRestart(state *container.State) bool {
	return state != nil &&
		state.Running &&
		state.Health != nil &&
		state.Health.Status != container.Starting &&
		state.Health.FailingStreak > 0
}

func (e *DockerEngine) ensureWorkerContainer(ctx context.Context, browser Browser, names generatedNames, browserContainerID string, restartWithBrowser bool) (string, error) {
	wantedNetworkMode := container.NetworkMode("container:" + browserContainerID)
	inspect, err := e.inspectOwnedContainer(ctx, names.worker, browser.ID, "worker")
	if err == nil && inspect.HostConfig != nil && inspect.HostConfig.NetworkMode != wantedNetworkMode {
		if err := e.removeOwnedContainer(ctx, names.worker, browser.ID, "worker"); err != nil {
			return "", fmt.Errorf("replace stale worker container: %w", err)
		}
		err = cerrdefs.ErrNotFound
	}
	if err == nil {
		if inspect.State == nil || !inspect.State.Running {
			if _, err := e.client.ContainerStart(ctx, inspect.ID, dockerclient.ContainerStartOptions{}); err != nil {
				return "", fmt.Errorf("start worker container: %w", err)
			}
		} else if restartWithBrowser {
			timeout := 10
			if _, err := e.client.ContainerRestart(ctx, inspect.ID, dockerclient.ContainerRestartOptions{Timeout: &timeout}); err != nil {
				return "", fmt.Errorf("reattach worker container after browser restart: %w", err)
			}
		}
		return inspect.ID, nil
	}
	if !cerrdefs.IsNotFound(err) {
		return "", err
	}

	initProcess := true
	pids := int64(128)
	environment := []string{
		"TZ=" + e.cfg.Timezone,
		"MCP_HOST=0.0.0.0",
		"MCP_PORT=8001",
		"MCP_CDP_ENDPOINT=http://127.0.0.1:9222",
		"MCP_ACTION_TIMEOUT_MS=10000",
		"MCP_NAVIGATION_TIMEOUT_MS=60000",
		"MCP_SESSION_IDLE_TIMEOUT_MS=1800000",
		"MCP_SNAPSHOT_MAX_CHARS=12000",
		"MCP_SNAPSHOT_MAX_ELEMENTS=150",
		"HUMAN_TAKEOVER_URL=http://" + names.browser + ":3000",
	}
	if e.cfg.WorkerAPIKey != "" {
		environment = append(environment, "MCP_API_KEY="+e.cfg.WorkerAPIKey)
	}
	created, err := e.client.ContainerCreate(ctx, dockerclient.ContainerCreateOptions{
		Name: names.worker,
		Config: &container.Config{
			Image:      e.cfg.WorkerImage,
			Entrypoint: []string{"/navego-worker"},
			Env:        environment,
			Labels:     runtimeLabels(browser, "worker"),
		},
		HostConfig: &container.HostConfig{
			NetworkMode:    wantedNetworkMode,
			RestartPolicy:  container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
			ReadonlyRootfs: true,
			Tmpfs:          map[string]string{"/tmp": "size=16m,mode=1777"},
			CapDrop:        []string{"ALL"},
			SecurityOpt:    []string{"no-new-privileges:true"},
			Init:           &initProcess,
			LogConfig:      managedLogConfig(),
			Resources: container.Resources{
				Memory:    256 << 20,
				NanoCPUs:  500_000_000,
				PidsLimit: &pids,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("create worker container: %w", err)
	}
	if _, err := e.client.ContainerStart(ctx, created.ID, dockerclient.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("start worker container: %w", err)
	}
	return created.ID, nil
}

func (e *DockerEngine) waitForBrowser(ctx context.Context, name string) error {
	waitCtx, cancel := context.WithTimeout(ctx, e.cfg.HealthWait)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		inspect, err := e.client.ContainerInspect(waitCtx, name, dockerclient.ContainerInspectOptions{})
		if err != nil {
			return fmt.Errorf("inspect browser health: %w", err)
		}
		state := inspect.Container.State
		if state == nil || !state.Running {
			return fmt.Errorf("browser exited before becoming healthy")
		}
		if state.Health == nil || state.Health.Status == container.Healthy {
			return nil
		}
		if state.Health.Status == container.Unhealthy {
			return fmt.Errorf("browser healthcheck failed")
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("wait for browser health: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func (e *DockerEngine) inspectOwnedContainer(ctx context.Context, name, browserID, role string) (container.InspectResponse, error) {
	result, err := e.client.ContainerInspect(ctx, name, dockerclient.ContainerInspectOptions{})
	if err != nil {
		return container.InspectResponse{}, err
	}
	if result.Container.Config == nil {
		return container.InspectResponse{}, fmt.Errorf("container %q has no configuration", name)
	}
	if err := validateLabels(result.Container.Config.Labels, browserID, role); err != nil {
		return container.InspectResponse{}, fmt.Errorf("refuse existing container %q: %w", name, err)
	}
	return result.Container, nil
}

func (e *DockerEngine) stopOwnedContainer(ctx context.Context, name, browserID, role string) error {
	inspect, err := e.inspectOwnedContainer(ctx, name, browserID, role)
	if cerrdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if inspect.State != nil && inspect.State.Running {
		timeout := 15
		if _, err := e.client.ContainerStop(ctx, inspect.ID, dockerclient.ContainerStopOptions{Timeout: &timeout}); err != nil && !cerrdefs.IsNotFound(err) {
			return fmt.Errorf("stop %s container: %w", role, err)
		}
	}
	return nil
}

func (e *DockerEngine) removeOwnedContainer(ctx context.Context, name, browserID, role string) error {
	inspect, err := e.inspectOwnedContainer(ctx, name, browserID, role)
	if cerrdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := e.stopOwnedContainer(ctx, name, browserID, role); err != nil {
		return err
	}
	if _, err := e.client.ContainerRemove(ctx, inspect.ID, dockerclient.ContainerRemoveOptions{Force: true}); err != nil && !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("remove %s container: %w", role, err)
	}
	return nil
}

func (e *DockerEngine) removeOwnedVolume(ctx context.Context, name, browserID string) error {
	inspect, err := e.client.VolumeInspect(ctx, name, dockerclient.VolumeInspectOptions{})
	if cerrdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect profile volume: %w", err)
	}
	if err := validateLabels(inspect.Volume.Labels, browserID, "profile"); err != nil {
		return fmt.Errorf("refuse existing volume %q: %w", name, err)
	}
	if _, err := e.client.VolumeRemove(ctx, name, dockerclient.VolumeRemoveOptions{}); err != nil && !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("remove profile volume: %w", err)
	}
	return nil
}

func validateLabels(labels map[string]string, browserID, role string) error {
	if labels[managedLabel] != "true" || labels[browserLabel] != browserID || labels[roleLabel] != role {
		return fmt.Errorf("resource is not owned by Navego browser %q with role %q", browserID, role)
	}
	return nil
}

func managedLogConfig() container.LogConfig {
	return container.LogConfig{Type: "json-file", Config: map[string]string{"max-size": "10m", "max-file": "3"}}
}

func cleanTitle(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "Browser"
	}
	runes := []rune(value)
	if len(runes) > 48 {
		return string(runes[:48])
	}
	return value
}
