package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type Runner struct {
	control      ControlPlane
	engine       Engine
	agentID      string
	pollInterval time.Duration
	logger       *slog.Logger
	telemetry    map[string]telemetryReport
}

type telemetryReport struct {
	title string
	url   string
	at    time.Time
}

const telemetryInterval = 15 * time.Second

func NewRunner(control ControlPlane, engine Engine, agentID string, pollInterval time.Duration, logger *slog.Logger) (*Runner, error) {
	agentID = strings.TrimSpace(agentID)
	if control == nil || engine == nil {
		return nil, fmt.Errorf("control plane and engine are required")
	}
	if agentID == "" {
		return nil, fmt.Errorf("agent ID is required")
	}
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		control:      control,
		engine:       engine,
		agentID:      agentID,
		pollInterval: pollInterval,
		logger:       logger,
		telemetry:    make(map[string]telemetryReport),
	}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	if err := r.Reconcile(ctx); err != nil {
		r.logger.Error("initial reconciliation failed", "error", err)
	}
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.Reconcile(ctx); err != nil {
				r.logger.Error("reconciliation failed", "error", err)
			}
		}
	}
}

func (r *Runner) Reconcile(ctx context.Context) error {
	commands, err := r.control.Commands(ctx, r.agentID)
	if err != nil {
		return err
	}
	for _, browser := range commands {
		if err := r.reconcileBrowser(ctx, browser); err != nil {
			r.logger.Error("browser reconciliation failed", "browser_id", browser.ID, "state", browser.State, "error", err)
		}
	}
	return nil
}

func (r *Runner) reconcileBrowser(ctx context.Context, browser Browser) error {
	switch browser.State {
	case "queued", "starting":
		if browser.State == "queued" {
			if err := r.control.Report(ctx, browser.ID, StateReport{AgentID: r.agentID, State: "starting"}); err != nil {
				return fmt.Errorf("claim browser: %w", err)
			}
		}
		runtime, err := r.engine.EnsureRunning(ctx, browser)
		if err != nil {
			r.reportError(ctx, browser, err)
			return fmt.Errorf("start runtime: %w", err)
		}
		report := reportForRuntime(r.agentID, "running", runtime)
		if err := r.control.Report(ctx, browser.ID, report); err != nil {
			return fmt.Errorf("report running state: %w", err)
		}
		r.rememberTelemetry(browser.ID, runtime)
	case "running", "error":
		runtime, err := r.engine.EnsureRunning(ctx, browser)
		if err != nil {
			r.reportError(ctx, browser, err)
			return fmt.Errorf("repair running runtime: %w", err)
		}
		if r.shouldReportTelemetry(browser.ID, runtime) {
			if err := r.control.Report(ctx, browser.ID, reportForRuntime(r.agentID, "running", runtime)); err != nil {
				return fmt.Errorf("report browser telemetry: %w", err)
			}
			r.rememberTelemetry(browser.ID, runtime)
		}
	case "stopping":
		if err := r.engine.EnsureStopped(ctx, browser); err != nil {
			r.reportError(ctx, browser, err)
			return fmt.Errorf("stop runtime: %w", err)
		}
		if err := r.control.Report(ctx, browser.ID, StateReport{AgentID: r.agentID, State: "stopped"}); err != nil {
			return fmt.Errorf("report stopped state: %w", err)
		}
	case "deleting":
		if err := r.engine.EnsureDeleted(ctx, browser); err != nil {
			return fmt.Errorf("delete runtime: %w", err)
		}
		if err := r.control.ConfirmDeletion(ctx, browser.ID); err != nil {
			return fmt.Errorf("confirm deletion: %w", err)
		}
	}
	return nil
}

func (r *Runner) reportError(ctx context.Context, browser Browser, cause error) {
	report := StateReport{
		AgentID:          r.agentID,
		State:            "error",
		BrowserContainer: browser.BrowserContainer,
		WorkerContainer:  browser.WorkerContainer,
		ProfileVolume:    browser.ProfileVolume,
		LastError:        safeError(cause),
	}
	if err := r.control.Report(ctx, browser.ID, report); err != nil {
		r.logger.Error("failed to report browser error", "browser_id", browser.ID, "error", err)
	}
}

func reportForRuntime(agentID, state string, runtime Runtime) StateReport {
	return StateReport{
		AgentID:          agentID,
		State:            state,
		BrowserContainer: runtime.BrowserContainer,
		WorkerContainer:  runtime.WorkerContainer,
		ProfileVolume:    runtime.ProfileVolume,
		WorkerEndpoint:   runtime.WorkerEndpoint,
		ViewerEndpoint:   runtime.ViewerEndpoint,
		Title:            runtime.Title,
		URL:              runtime.URL,
	}
}

func (r *Runner) shouldReportTelemetry(browserID string, runtime Runtime) bool {
	previous, ok := r.telemetry[browserID]
	if !ok || previous.title != runtime.Title || previous.url != runtime.URL {
		return true
	}
	return time.Since(previous.at) >= telemetryInterval
}

func (r *Runner) rememberTelemetry(browserID string, runtime Runtime) {
	r.telemetry[browserID] = telemetryReport{
		title: runtime.Title,
		url:   runtime.URL,
		at:    time.Now(),
	}
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	const max = 1000
	if len(value) <= max {
		return value
	}
	return value[:max]
}
