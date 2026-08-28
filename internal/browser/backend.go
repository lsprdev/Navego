package browser

import (
	"context"
	"fmt"
	"strings"
)

type BackendMode string

const (
	BackendModeCurrent  BackendMode = ""
	BackendModeAuto     BackendMode = "auto"
	BackendModeObscura  BackendMode = "obscura"
	BackendModeChromium BackendMode = "chromium"
)

// ParseBackendMode accepts the short prefixes used in chat as well as the
// canonical backend names exposed in structured MCP results. An empty value
// means that the caller wants to preserve the currently selected mode.
func ParseBackendMode(value string) (BackendMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return BackendModeCurrent, nil
	case "auto":
		return BackendModeAuto, nil
	case "ob", "obscura":
		return BackendModeObscura, nil
	case "ch", "chromium":
		return BackendModeChromium, nil
	default:
		return BackendModeCurrent, fmt.Errorf("invalid browser backend %q: use auto, ob/obscura, or ch/chromium", value)
	}
}

// BackendController is implemented by the hybrid router. It allows MCP calls
// to make routing explicit instead of relying on URL heuristics alone.
type BackendController interface {
	OpenWithBackend(context.Context, string, BackendMode) (Snapshot, error)
	SelectBackend(context.Context, BackendMode) (Snapshot, error)
}
