package browser

import (
	"errors"
	"log/slog"
	"net/url"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/target"
)

func logBlockedRequest(request *fetch.EventRequestPaused, targetID target.ID, err error, elapsed time.Duration) {
	code := "policy_error"
	var policyErr *PolicyError
	if errors.As(err, &policyErr) {
		code = policyErr.Code
	}
	host := ""
	if parsed, parseErr := url.Parse(request.Request.URL); parseErr == nil {
		host = parsed.Hostname()
	}
	// Never log the raw error/URL: either may include login tickets, userinfo,
	// query strings or form data. A reason code and target correlate failures.
	slog.Warn("browser request blocked", "reason", code, "host", host,
		"target_id", string(targetID), "resource_type", string(request.ResourceType),
		"duration_ms", elapsed.Milliseconds())
}
