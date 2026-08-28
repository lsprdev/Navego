package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/cdproto/target"
)

const maxTargetListBytes = 1 << 20

type debuggerTarget struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func discoverInitialTarget(ctx context.Context, endpoint string) (target.ID, error) {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("CDP endpoint does not support HTTP target discovery")
	}
	u.Path = "/json/list"
	u.RawQuery = ""
	u.Fragment = ""
	discoveryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(discoveryCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create CDP target discovery request: %w", err)
	}
	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
	if err != nil {
		return "", fmt.Errorf("discover CDP targets: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("discover CDP targets: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxTargetListBytes+1))
	if err != nil {
		return "", fmt.Errorf("read CDP targets: %w", err)
	}
	if len(data) > maxTargetListBytes {
		return "", errors.New("CDP target list exceeded the size limit")
	}
	var targets []debuggerTarget
	if err := json.Unmarshal(data, &targets); err != nil {
		return "", fmt.Errorf("decode CDP targets: %w", err)
	}
	return chooseInitialTarget(targets)
}

func chooseInitialTarget(targets []debuggerTarget) (target.ID, error) {
	var fallback target.ID
	for _, candidate := range targets {
		if candidate.Type != "page" || strings.TrimSpace(candidate.ID) == "" {
			continue
		}
		if fallback == "" {
			fallback = target.ID(candidate.ID)
		}
		if !isBlankTarget(candidate.URL) {
			return target.ID(candidate.ID), nil
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", errors.New("CDP target discovery returned no page targets")
}

func isBlankTarget(rawURL string) bool {
	rawURL = strings.ToLower(strings.TrimSpace(rawURL))
	return rawURL == "" || rawURL == "about:blank" || strings.HasPrefix(rawURL, "chrome://newtab") || strings.HasPrefix(rawURL, "https://duckduckgo.com/chrome_newtab")
}
