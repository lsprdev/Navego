package agent

import "context"

type Browser struct {
	ID               string `json:"id"`
	OwnerID          string `json:"owner_id"`
	Name             string `json:"name"`
	State            string `json:"state"`
	BrowserContainer string `json:"browser_container"`
	WorkerContainer  string `json:"worker_container"`
	ProfileVolume    string `json:"profile_volume"`
}

type Runtime struct {
	BrowserContainer string `json:"browser_container,omitempty"`
	WorkerContainer  string `json:"worker_container,omitempty"`
	ProfileVolume    string `json:"profile_volume,omitempty"`
	WorkerEndpoint   string `json:"worker_endpoint,omitempty"`
	ViewerEndpoint   string `json:"viewer_endpoint,omitempty"`
	Title            string `json:"title,omitempty"`
	URL              string `json:"url,omitempty"`
}

type StateReport struct {
	AgentID          string `json:"agent_id"`
	State            string `json:"state"`
	BrowserContainer string `json:"browser_container,omitempty"`
	WorkerContainer  string `json:"worker_container,omitempty"`
	ProfileVolume    string `json:"profile_volume,omitempty"`
	WorkerEndpoint   string `json:"worker_endpoint,omitempty"`
	ViewerEndpoint   string `json:"viewer_endpoint,omitempty"`
	Title            string `json:"title,omitempty"`
	URL              string `json:"url,omitempty"`
	LastError        string `json:"last_error,omitempty"`
}

type ControlPlane interface {
	Commands(context.Context, string) ([]Browser, error)
	Report(context.Context, string, StateReport) error
	ConfirmDeletion(context.Context, string) error
}

type Engine interface {
	EnsureRunning(context.Context, Browser) (Runtime, error)
	EnsureStopped(context.Context, Browser) error
	EnsureDeleted(context.Context, Browser) error
}
