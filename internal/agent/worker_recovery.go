package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"
)

// Longer than a normal 60s navigation plus its snapshot. A single busy health
// probe must never restart a worker in the middle of a user action.
const workerFailureGrace = 2 * time.Minute
const workerRestartCooldown = 2 * time.Minute

type workerRecovery struct {
	workerID     string
	startedAt    string
	firstFailure time.Time
	lastAttempt  time.Time
	failures     int
}

func workerPredatesBrowser(worker, browser *container.State) bool {
	if worker == nil || browser == nil || !worker.Running || !browser.Running {
		return false
	}
	w, wErr := time.Parse(time.RFC3339Nano, worker.StartedAt)
	b, bErr := time.Parse(time.RFC3339Nano, browser.StartedAt)
	return wErr == nil && bErr == nil && !w.IsZero() && !b.IsZero() && w.Before(b)
}

func (r *workerRecovery) failed(workerID, startedAt string, now time.Time) bool {
	if r.workerID != workerID {
		*r = workerRecovery{workerID: workerID}
	}
	if r.startedAt != startedAt {
		r.startedAt = startedAt
		r.firstFailure = time.Time{}
		r.failures = 0
	}
	if r.firstFailure.IsZero() {
		r.firstFailure = now
	}
	r.failures++
	return r.failures >= 3 && now.Sub(r.firstFailure) >= workerFailureGrace &&
		(r.lastAttempt.IsZero() || now.Sub(r.lastAttempt) >= workerRestartCooldown)
}

func (e *DockerEngine) forgetWorkerRecovery(browserID string) {
	e.recoveryMu.Lock()
	defer e.recoveryMu.Unlock()
	delete(e.recovery, browserID)
}

func (e *DockerEngine) recoverWorker(ctx context.Context, browser Browser, names generatedNames, healthErr error, now time.Time) error {
	e.recoveryMu.Lock()
	defer e.recoveryMu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if healthErr == nil {
		delete(e.recovery, browser.ID)
		return nil
	}
	// Re-inspect and validate ownership: never restart an arbitrary container
	// based only on a stale endpoint or a failed HTTP request.
	inspect, err := e.inspectOwnedContainer(ctx, names.worker, browser.ID, "worker")
	if err != nil {
		return fmt.Errorf("inspect unresponsive worker: %w", err)
	}
	if inspect.State == nil || !inspect.State.Running {
		delete(e.recovery, browser.ID)
		return nil // ensureWorkerContainer will start it on the next pass.
	}
	if e.recovery == nil {
		e.recovery = make(map[string]workerRecovery)
	}
	r := e.recovery[browser.ID]
	due := r.failed(inspect.ID, inspect.State.StartedAt, now)
	if due {
		// Record attempts too, so Docker errors cannot cause a restart storm.
		r.lastAttempt = now
		r.firstFailure = time.Time{}
		r.failures = 0
	}
	e.recovery[browser.ID] = r
	if !due {
		return nil
	}
	slog.Warn("restarting unresponsive browser worker", "browser_id", browser.ID, "worker_id", inspect.ID)
	restartCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	timeout := 10
	if _, err := e.client.ContainerRestart(restartCtx, inspect.ID, dockerclient.ContainerRestartOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("recover unresponsive worker: %w", err)
	}
	// No browser/profile changes and no replay of MCP calls: an interrupted
	// write might already have taken effect and must not run a second time.
	return nil
}
