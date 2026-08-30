package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeControl struct {
	commands  []Browser
	reports   []StateReport
	deletedID string
}

func (f *fakeControl) Commands(context.Context, string) ([]Browser, error) {
	return f.commands, nil
}

func (f *fakeControl) Report(_ context.Context, _ string, report StateReport) error {
	f.reports = append(f.reports, report)
	return nil
}

func (f *fakeControl) ConfirmDeletion(_ context.Context, id string) error {
	f.deletedID = id
	return nil
}

type fakeEngine struct {
	runtime Runtime
	err     error
	started []string
	stopped []string
	deleted []string
}

func (f *fakeEngine) EnsureRunning(_ context.Context, browser Browser) (Runtime, error) {
	f.started = append(f.started, browser.ID)
	return f.runtime, f.err
}

func (f *fakeEngine) EnsureStopped(_ context.Context, browser Browser) error {
	f.stopped = append(f.stopped, browser.ID)
	return f.err
}

func (f *fakeEngine) EnsureDeleted(_ context.Context, browser Browser) error {
	f.deleted = append(f.deleted, browser.ID)
	return f.err
}

func TestRunnerStartsQueuedBrowser(t *testing.T) {
	control := &fakeControl{commands: []Browser{{ID: "browser1", State: "queued"}}}
	engine := &fakeEngine{runtime: Runtime{
		BrowserContainer: "browser-container",
		WorkerContainer:  "worker-container",
		ProfileVolume:    "profile-volume",
		WorkerEndpoint:   "http://browser:8001",
		ViewerEndpoint:   "http://browser:3000",
	}}
	runner, err := NewRunner(control, engine, "agent-1", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(engine.started) != 1 || engine.started[0] != "browser1" {
		t.Fatalf("unexpected starts: %#v", engine.started)
	}
	if len(control.reports) != 2 || control.reports[0].State != "starting" || control.reports[1].State != "running" {
		t.Fatalf("unexpected reports: %#v", control.reports)
	}
	if control.reports[1].WorkerEndpoint != "http://browser:8001" {
		t.Fatalf("runtime was not reported: %#v", control.reports[1])
	}
}

func TestRunnerRepairsRunningBrowserWithoutRewritingItsState(t *testing.T) {
	control := &fakeControl{commands: []Browser{{ID: "browser1", State: "running"}}}
	engine := &fakeEngine{runtime: Runtime{
		BrowserContainer: "browser-container",
		Title:            "Example",
		URL:              "https://example.com",
	}}
	runner, err := NewRunner(control, engine, "agent-1", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(engine.started) != 1 || engine.started[0] != "browser1" {
		t.Fatalf("running browser was not reconciled: %#v", engine.started)
	}
	if len(control.reports) != 1 || control.reports[0].Title != "Example" || control.reports[0].URL != "https://example.com" {
		t.Fatalf("browser telemetry was not reported: %#v", control.reports)
	}
	if err := runner.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(control.reports) != 1 {
		t.Fatalf("unchanged telemetry should be throttled: %#v", control.reports)
	}
}

func TestRunnerRecoversBrowserFromTransientError(t *testing.T) {
	control := &fakeControl{commands: []Browser{{ID: "browser1", State: "error"}}}
	engine := &fakeEngine{runtime: Runtime{
		BrowserContainer: "browser-container",
		WorkerContainer:  "replacement-worker",
		WorkerEndpoint:   "http://browser:8001",
		ViewerEndpoint:   "http://browser:3000",
	}}
	runner, err := NewRunner(control, engine, "agent-1", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(engine.started) != 1 || engine.started[0] != "browser1" {
		t.Fatalf("errored browser was not reconciled: %#v", engine.started)
	}
	if len(control.reports) != 1 || control.reports[0].State != "running" || control.reports[0].WorkerContainer != "replacement-worker" {
		t.Fatalf("browser recovery was not reported: %#v", control.reports)
	}
}

func TestRunnerStopsAndDeletesBrowsers(t *testing.T) {
	control := &fakeControl{commands: []Browser{
		{ID: "stop-me", State: "stopping"},
		{ID: "delete-me", State: "deleting"},
	}}
	engine := &fakeEngine{}
	runner, err := NewRunner(control, engine, "agent-1", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(engine.stopped) != 1 || engine.stopped[0] != "stop-me" {
		t.Fatalf("unexpected stops: %#v", engine.stopped)
	}
	if len(engine.deleted) != 1 || engine.deleted[0] != "delete-me" || control.deletedID != "delete-me" {
		t.Fatalf("unexpected deletion: engine=%#v control=%q", engine.deleted, control.deletedID)
	}
	if len(control.reports) != 1 || control.reports[0].State != "stopped" {
		t.Fatalf("unexpected reports: %#v", control.reports)
	}
}

func TestRunnerReportsProvisioningError(t *testing.T) {
	control := &fakeControl{commands: []Browser{{ID: "browser1", State: "starting"}}}
	engine := &fakeEngine{err: errors.New("image is unavailable")}
	runner, err := NewRunner(control, engine, "agent-1", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(control.reports) != 1 || control.reports[0].State != "error" || control.reports[0].LastError == "" {
		t.Fatalf("unexpected error report: %#v", control.reports)
	}
}
