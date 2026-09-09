package control

import (
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pocketbase/pocketbase/core"
)

// Only diagnostic fields cross the activity API. Never expose arbitrary audit
// metadata, tool arguments, page snapshots or successful tool output.
type activityDiagnostics struct {
	Error      string `json:"error,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Stage      string `json:"stage,omitempty"`
	DurationMS *int64 `json:"duration_ms,omitempty"`
}

var diagnosticURL = regexp.MustCompile(`(?i)(?:https?|wss?)://[^\s<>"']+`)
var diagnosticSecret = regexp.MustCompile(`(?i)(?:(?:bearer|basic)\s+[^\s,"']+|(?:password|passwd|senha|token|secret|authorization|cookie|api[_-]?key)\s*["']?\s*[:=]\s*(?:(?:bearer|basic)\s+[^\s,"';]+|"[^"]*"|'[^']*'|[^\s,;]+))`)
var diagnosticCode = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,80}$`)

func sanitizeDiagnostic(message string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[omitido]")
		}
	}
	message = diagnosticURL.ReplaceAllStringFunc(message, func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "[URL omitida]"
		}
		// Paths can carry viewer tickets; queries/fragments can carry tokens.
		return u.Scheme + "://" + u.Host + "/[omitido]"
	})
	message = diagnosticSecret.ReplaceAllString(message, "[credencial omitida]")
	runes := []rune(strings.TrimSpace(message))
	if len(runes) > 4000 {
		return string(runes[:4000]) + "… [truncado]"
	}
	return string(runes)
}

func diagnosticKind(message, stage string) string {
	m := strings.ToLower(message)
	timeout := strings.Contains(m, "timeout") || strings.Contains(m, "deadline exceeded") || strings.Contains(m, "timed out")
	switch {
	case timeout && (strings.Contains(m, "lookup ") || strings.Contains(m, "resolve ")):
		return "dns_timeout"
	case timeout:
		return "timeout"
	case strings.Contains(m, "context canceled") || strings.Contains(m, "context cancelled"):
		return "canceled"
	case stage == "transport":
		return "transport"
	default:
		return "tool_error"
	}
}

func toolFailureText(result *mcp.CallToolResult) string {
	if result == nil || !result.IsError {
		return ""
	}
	var lines []string
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok && !strings.HasPrefix(text.Text, "Chromium usado: ") {
			lines = append(lines, text.Text)
		}
	}
	return strings.Join(lines, "\n")
}

func (s *multiBrowserMCP) auditToolCall(ownerID, browserID, tool, stage string, started time.Time, result *mcp.CallToolResult, arguments map[string]any) {
	duration := time.Since(started).Milliseconds()
	metadata := map[string]any{"stage": stage, "duration_ms": duration}
	status := "success"
	if result == nil || result.IsError {
		status = "error"
		message := toolFailureText(result)
		if message == "" {
			message = "A ferramenta não retornou detalhes da falha."
		}
		metadata["kind"] = diagnosticKind(message, stage)
		secrets := []string{s.cfg.WorkerAPIKey, s.cfg.AgentToken, s.cfg.VaultKey}
		// User-provided text may be echoed by a validation error. Redact input
		// strings recursively; do not persist the arguments themselves.
		var collect func(any)
		collect = func(value any) {
			switch value := value.(type) {
			case string:
				if value != "" {
					secrets = append(secrets, value)
				}
			case map[string]any:
				for _, item := range value {
					collect(item)
				}
			case []any:
				for _, item := range value {
					collect(item)
				}
			}
		}
		collect(arguments)
		metadata["error"] = sanitizeDiagnostic(message, secrets...)
		if result != nil {
			if fields, ok := result.StructuredContent.(map[string]any); ok {
				if code, ok := fields["error_code"].(string); ok && diagnosticCode.MatchString(code) {
					metadata["error_code"] = code
				}
			}
		}
	}
	writeAudit(s.app, ownerID, browserID, "mcp."+tool, status, metadata)
}

func readActivityDiagnostics(record *core.Record) *activityDiagnostics {
	var details activityDiagnostics
	if err := record.UnmarshalJSONField("metadata", &details); err != nil {
		return nil
	}
	details.Error = sanitizeDiagnostic(details.Error)
	if !diagnosticCode.MatchString(details.ErrorCode) {
		details.ErrorCode = ""
	}
	if !diagnosticCode.MatchString(details.Stage) {
		details.Stage = ""
	}
	if !diagnosticCode.MatchString(details.Kind) {
		details.Kind = ""
	}
	if details.DurationMS != nil && *details.DurationMS < 0 {
		details.DurationMS = nil
	}
	if details.Error != "" && details.Kind == "" {
		details.Kind = diagnosticKind(details.Error, details.Stage)
	}
	if details.Error == "" && details.ErrorCode == "" && details.Stage == "" && details.Kind == "" && details.DurationMS == nil {
		return nil
	}
	return &details
}
