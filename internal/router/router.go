package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lsprdev/Navego/internal/browser"
	"github.com/lsprdev/Navego/internal/obscura"
)

const (
	defaultFailureThreshold = 3
	defaultCircuitCooldown  = 30 * time.Second
)

type publicBackend interface {
	Open(context.Context, string, string, int) (obscura.Snapshot, error)
	Snapshot(context.Context, int) (obscura.Snapshot, error)
	Markdown(context.Context, int) (string, error)
	Links(context.Context, bool, int) ([]obscura.Link, error)
	Screenshot(context.Context, int, int) ([]byte, string, error)
	PDF(context.Context) ([]byte, string, error)
	Close() error
}

type metadataFetcher interface {
	Fetch(context.Context, string) (browser.Metadata, error)
}

type metadataLinkFetcher interface {
	Links(context.Context, string, int) ([]browser.PageLink, error)
}

type Option func(*Router)

func WithMetadataFetcher(fetcher metadataFetcher) Option {
	return func(router *Router) {
		router.metadata = fetcher
	}
}

func WithCircuitBreaker(failureThreshold int, cooldown time.Duration) Option {
	return func(router *Router) {
		if failureThreshold > 0 {
			router.failureThreshold = failureThreshold
		}
		if cooldown > 0 {
			router.circuitCooldown = cooldown
		}
	}
}

type Backend string

const (
	BackendChromium Backend = "chromium"
	BackendObscura  Backend = "obscura"
)

type Router struct {
	mu sync.Mutex

	chromium            browser.Controller
	public              publicBackend
	active              Backend
	selection           browser.BackendMode
	pinnedChromiumHosts map[string]struct{}
	publicSnapshot      obscura.Snapshot
	publicMetadata      browser.Metadata
	generation          uint64
	alwaysChromium      []string
	maxChars            int
	logger              *slog.Logger
	metadata            metadataFetcher
	failureThreshold    int
	circuitCooldown     time.Duration
	consecutiveFailures int
	circuitOpenUntil    time.Time
	halfOpenProbe       bool
	now                 func() time.Time
}

func New(chromium browser.Controller, public publicBackend, alwaysChromium []string, maxChars int, logger *slog.Logger, options ...Option) *Router {
	domains := make([]string, 0, len(alwaysChromium))
	for _, domain := range alwaysChromium {
		domain = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(domain, ".")))
		if domain != "" {
			domains = append(domains, domain)
		}
	}
	router := &Router{
		chromium:            chromium,
		public:              public,
		active:              BackendChromium,
		selection:           browser.BackendModeAuto,
		pinnedChromiumHosts: make(map[string]struct{}),
		alwaysChromium:      domains,
		maxChars:            maxChars,
		logger:              logger,
		failureThreshold:    defaultFailureThreshold,
		circuitCooldown:     defaultCircuitCooldown,
		now:                 time.Now,
	}
	for _, option := range options {
		option(router)
	}
	return router
}

func (r *Router) Status(ctx context.Context) (browser.Status, error) {
	r.mu.Lock()
	active := r.active
	publicSnapshot := r.publicSnapshot
	r.mu.Unlock()
	if active == BackendObscura {
		return browser.Status{
			Connected: true,
			URL:       publicSnapshot.URL,
			Title:     publicSnapshot.Title,
			Backend:   string(BackendObscura),
			Routing:   r.routingStatus(),
		}, nil
	}
	status, err := r.chromium.Status(ctx)
	if err == nil {
		status.Routing = r.routingStatus()
	}
	return status, err
}

func (r *Router) Open(ctx context.Context, rawURL string) (browser.Snapshot, error) {
	return r.OpenWithBackend(ctx, rawURL, browser.BackendModeCurrent)
}

func (r *Router) OpenWithBackend(ctx context.Context, rawURL string, requested browser.BackendMode) (browser.Snapshot, error) {
	mode, err := browser.ParseBackendMode(string(requested))
	if err != nil {
		return browser.Snapshot{}, err
	}
	if mode == browser.BackendModeCurrent {
		mode = r.selectedMode()
	} else {
		r.setSelection(mode)
	}

	switch mode {
	case browser.BackendModeChromium:
		return r.openChromium(ctx, rawURL)
	case browser.BackendModeObscura:
		return r.openObscura(ctx, rawURL, false)
	case browser.BackendModeAuto:
		if r.public == nil || r.requiresChromium(rawURL) {
			return r.openChromium(ctx, rawURL)
		}
		return r.openObscura(ctx, rawURL, true)
	default:
		return browser.Snapshot{}, fmt.Errorf("unsupported browser backend mode %q", mode)
	}

}

func (r *Router) SelectBackend(ctx context.Context, requested browser.BackendMode) (browser.Snapshot, error) {
	mode, err := browser.ParseBackendMode(string(requested))
	if err != nil {
		return browser.Snapshot{}, err
	}
	if mode == browser.BackendModeCurrent {
		return browser.Snapshot{}, errors.New("backend is required: use auto, ob/obscura, or ch/chromium")
	}
	r.setSelection(mode)

	switch mode {
	case browser.BackendModeAuto:
		return r.Snapshot(ctx)
	case browser.BackendModeChromium:
		if r.activeBackend() == BackendChromium {
			return r.chromium.Snapshot(ctx)
		}
		return r.handoffCurrentToChromium(ctx)
	case browser.BackendModeObscura:
		if r.activeBackend() == BackendObscura {
			return r.Snapshot(ctx)
		}
		status, statusErr := r.chromium.Status(ctx)
		if statusErr != nil {
			return browser.Snapshot{}, fmt.Errorf("read current Chromium page before switching to Obscura: %w", statusErr)
		}
		if strings.TrimSpace(status.URL) == "" {
			return browser.Snapshot{}, errors.New("Chromium has no current URL to open in Obscura")
		}
		return r.openObscura(ctx, status.URL, false)
	default:
		return browser.Snapshot{}, fmt.Errorf("unsupported browser backend mode %q", mode)
	}
}

func (r *Router) openChromium(ctx context.Context, rawURL string) (browser.Snapshot, error) {
	snapshot, err := r.chromium.Open(ctx, rawURL)
	if err == nil {
		r.setActive(BackendChromium, obscura.Snapshot{}, browser.Metadata{})
	}
	return snapshot, err
}

func (r *Router) openObscura(ctx context.Context, rawURL string, allowFallback bool) (browser.Snapshot, error) {
	if r.public == nil {
		if allowFallback {
			return r.openChromium(ctx, rawURL)
		}
		return browser.Snapshot{}, errors.New("Obscura backend is not configured")
	}
	if !r.publicAttemptAllowed() {
		if !allowFallback {
			return browser.Snapshot{}, errors.New("Obscura was explicitly selected but its circuit breaker is open")
		}
		if r.logger != nil {
			r.logger.Warn("Obscura circuit is open; routing to Chromium", "url", rawURL)
		}
		return r.openChromium(ctx, rawURL)
	}

	publicSnapshot, err := r.public.Open(ctx, rawURL, "load", r.maxChars)
	if err == nil {
		r.recordPublicSuccess()
		markdown, markdownErr := r.public.Markdown(ctx, r.maxChars)
		if markdownErr == nil && strings.TrimSpace(markdown) != "" {
			publicSnapshot.Text += "\n\nMarkdown:\n" + markdown
		}
		r.setActive(BackendObscura, publicSnapshot, r.fetchMetadata(ctx, publicSnapshot.URL))
		return r.convertPublic(publicSnapshot), nil
	}
	r.recordPublicFailure()
	if !allowFallback {
		return browser.Snapshot{}, fmt.Errorf("Obscura was explicitly selected and could not open the page: %w", err)
	}
	if r.logger != nil {
		r.logger.Warn("Obscura open failed; falling back to Chromium", "url", rawURL, "error", err)
	}
	chromiumSnapshot, chromiumErr := r.openChromium(ctx, rawURL)
	if chromiumErr != nil {
		return browser.Snapshot{}, fmt.Errorf("Obscura failed (%v) and Chromium fallback failed: %w", err, chromiumErr)
	}
	return chromiumSnapshot, nil
}

func (r *Router) Snapshot(ctx context.Context) (browser.Snapshot, error) {
	if r.activeBackend() == BackendChromium {
		return r.chromium.Snapshot(ctx)
	}
	if !r.publicAttemptAllowed() {
		return r.fallbackCurrentToChromium(ctx, errors.New("Obscura circuit is open"))
	}
	publicSnapshot, err := r.public.Snapshot(ctx, r.maxChars)
	if err != nil {
		r.recordPublicFailure()
		return r.fallbackCurrentToChromium(ctx, err)
	}
	r.recordPublicSuccess()
	markdown, markdownErr := r.public.Markdown(ctx, r.maxChars)
	if markdownErr == nil && strings.TrimSpace(markdown) != "" {
		publicSnapshot.Text += "\n\nMarkdown:\n" + markdown
	}
	metadata := r.currentPublicMetadata(publicSnapshot.URL)
	if metadata.Empty() {
		metadata = r.fetchMetadata(ctx, publicSnapshot.URL)
	}
	r.setActive(BackendObscura, publicSnapshot, metadata)
	return r.convertPublic(publicSnapshot), nil
}

func (r *Router) Find(ctx context.Context, query string, limit int) (browser.FindResult, error) {
	query, limit, err := browser.NormalizeFindRequest(query, limit)
	if err != nil {
		return browser.FindResult{}, err
	}
	if r.activeBackend() == BackendChromium {
		finder, ok := r.chromium.(browser.Finder)
		if !ok {
			return browser.FindResult{}, errors.New("Chromium backend does not support find")
		}
		return finder.Find(ctx, query, limit)
	}
	if !r.publicAttemptAllowed() {
		if _, err := r.fallbackCurrentToChromium(ctx, errors.New("Obscura circuit is open")); err != nil {
			return browser.FindResult{}, err
		}
		return r.Find(ctx, query, limit)
	}
	publicSnapshot, err := r.public.Snapshot(ctx, r.maxChars)
	if err != nil {
		r.recordPublicFailure()
		if _, fallbackErr := r.fallbackCurrentToChromium(ctx, err); fallbackErr != nil {
			return browser.FindResult{}, fallbackErr
		}
		return r.Find(ctx, query, limit)
	}
	r.recordPublicSuccess()
	metadata := r.currentPublicMetadata(publicSnapshot.URL)
	if metadata.Empty() {
		metadata = r.fetchMetadata(ctx, publicSnapshot.URL)
	}
	r.setActive(BackendObscura, publicSnapshot, metadata)
	snapshot := r.convertPublic(publicSnapshot)
	result, err := browser.FindSnapshot(snapshot, query, limit)
	if err != nil {
		return browser.FindResult{}, err
	}
	links, linkErr := r.public.Links(ctx, false, 200)
	if linkErr != nil {
		if r.logger != nil {
			r.logger.Debug("Obscura links unavailable during find", "error", linkErr)
		}
		return result, nil
	}
	lowerQuery := strings.ToLower(query)
	linkMatches := make([]browser.FindMatch, 0)
	seen := make(map[string]struct{})
	for _, link := range links {
		if !strings.Contains(strings.ToLower(link.Text+" "+link.Href), lowerQuery) {
			continue
		}
		key := link.Text + "\x00" + link.Href
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		linkMatches = append(linkMatches, browser.FindMatch{Text: link.Text, Href: link.Href})
	}
	if len(linkMatches) == 0 {
		if fallback, ok := r.metadata.(metadataLinkFetcher); ok {
			fallbackLinks, fallbackErr := fallback.Links(ctx, publicSnapshot.URL, 200)
			if fallbackErr != nil {
				if r.logger != nil {
					r.logger.Debug("public HTML links unavailable during find", "error", fallbackErr)
				}
			} else {
				for _, link := range fallbackLinks {
					if strings.Contains(strings.ToLower(link.Text+" "+link.Href), lowerQuery) {
						linkMatches = append(linkMatches, browser.FindMatch{Text: link.Text, Href: link.Href})
					}
				}
			}
		}
	}
	combined := append(linkMatches, result.Matches...)
	if len(combined) > limit {
		combined = combined[:limit]
		result.Truncated = true
	}
	result.Matches = combined
	return result, nil
}

func (r *Router) Wait(ctx context.Context, condition browser.WaitCondition) (browser.Snapshot, error) {
	condition, err := browser.NormalizeWaitCondition(condition)
	if err != nil {
		return browser.Snapshot{}, err
	}
	if r.activeBackend() == BackendChromium {
		waiter, ok := r.chromium.(browser.Waiter)
		if !ok {
			return browser.Snapshot{}, errors.New("Chromium backend does not support wait")
		}
		return waiter.Wait(ctx, condition)
	}
	waitCtx, cancel := context.WithTimeout(ctx, condition.Timeout)
	defer cancel()
	if !r.publicAttemptAllowed() {
		if _, err := r.fallbackCurrentToChromium(waitCtx, errors.New("Obscura circuit is open")); err != nil {
			return browser.Snapshot{}, err
		}
		waiter, ok := r.chromium.(browser.Waiter)
		if !ok {
			return browser.Snapshot{}, errors.New("Chromium backend does not support wait")
		}
		return waiter.Wait(waitCtx, condition)
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		publicSnapshot, snapshotErr := r.public.Snapshot(waitCtx, r.maxChars)
		if snapshotErr != nil {
			r.recordPublicFailure()
			if _, fallbackErr := r.fallbackCurrentToChromium(waitCtx, snapshotErr); fallbackErr != nil {
				return browser.Snapshot{}, fallbackErr
			}
			waiter, ok := r.chromium.(browser.Waiter)
			if !ok {
				return browser.Snapshot{}, errors.New("Chromium backend does not support wait")
			}
			return waiter.Wait(waitCtx, condition)
		}
		r.recordPublicSuccess()
		candidate := browser.Snapshot{URL: publicSnapshot.URL, Text: publicSnapshot.Text}
		if browser.WaitSatisfied(candidate, condition) {
			markdown, markdownErr := r.public.Markdown(waitCtx, r.maxChars)
			if markdownErr == nil && strings.TrimSpace(markdown) != "" {
				publicSnapshot.Text += "\n\nMarkdown:\n" + markdown
			}
			metadata := r.currentPublicMetadata(publicSnapshot.URL)
			if metadata.Empty() {
				metadata = r.fetchMetadata(waitCtx, publicSnapshot.URL)
			}
			r.setActive(BackendObscura, publicSnapshot, metadata)
			return r.convertPublic(publicSnapshot), nil
		}
		select {
		case <-waitCtx.Done():
			return browser.Snapshot{}, fmt.Errorf("wait condition was not met: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func (r *Router) ListTabs(ctx context.Context) (browser.TabsResult, error) {
	if err := r.requireChromiumInteraction(); err != nil {
		return browser.TabsResult{}, err
	}
	tabs, ok := r.chromium.(browser.TabController)
	if !ok {
		return browser.TabsResult{}, errors.New("Chromium backend does not support tabs")
	}
	return tabs.ListTabs(ctx)
}

func (r *Router) NewTab(ctx context.Context, rawURL string) (browser.Snapshot, error) {
	if err := r.requireChromiumInteraction(); err != nil {
		return browser.Snapshot{}, err
	}
	tabs, ok := r.chromium.(browser.TabController)
	if !ok {
		return browser.Snapshot{}, errors.New("Chromium backend does not support tabs")
	}
	snapshot, err := tabs.NewTab(ctx, rawURL)
	if err == nil {
		r.setActive(BackendChromium, obscura.Snapshot{}, browser.Metadata{})
	}
	return snapshot, err
}

func (r *Router) SwitchTab(ctx context.Context, tabID string) (browser.Snapshot, error) {
	if err := r.requireChromiumInteraction(); err != nil {
		return browser.Snapshot{}, err
	}
	tabs, ok := r.chromium.(browser.TabController)
	if !ok {
		return browser.Snapshot{}, errors.New("Chromium backend does not support tabs")
	}
	snapshot, err := tabs.SwitchTab(ctx, tabID)
	if err == nil {
		r.setActive(BackendChromium, obscura.Snapshot{}, browser.Metadata{})
	}
	return snapshot, err
}

func (r *Router) CloseTab(ctx context.Context, tabID string) (browser.Snapshot, error) {
	if err := r.requireChromiumInteraction(); err != nil {
		return browser.Snapshot{}, err
	}
	tabs, ok := r.chromium.(browser.TabController)
	if !ok {
		return browser.Snapshot{}, errors.New("Chromium backend does not support tabs")
	}
	snapshot, err := tabs.CloseTab(ctx, tabID)
	if err == nil {
		r.setActive(BackendChromium, obscura.Snapshot{}, browser.Metadata{})
	}
	return snapshot, err
}

func (r *Router) Click(ctx context.Context, ref string) (browser.Snapshot, error) {
	if err := r.requireChromiumInteraction(); err != nil {
		return browser.Snapshot{}, err
	}
	return r.chromium.Click(ctx, ref)
}

func (r *Router) Type(ctx context.Context, ref, text string, clear bool) (browser.Snapshot, error) {
	if err := r.requireChromiumInteraction(); err != nil {
		return browser.Snapshot{}, err
	}
	return r.chromium.Type(ctx, ref, text, clear)
}

func (r *Router) Screenshot(ctx context.Context, fullPage bool) ([]byte, string, error) {
	if r.activeBackend() == BackendChromium {
		return r.chromium.Screenshot(ctx, fullPage)
	}
	if fullPage {
		return nil, "", errors.New("full-page PNG is not available in Obscura; use browser_export_pdf or hand off to Chromium")
	}
	return r.public.Screenshot(ctx, 0, 0)
}

func (r *Router) PDF(ctx context.Context) ([]byte, string, error) {
	if r.activeBackend() == BackendChromium {
		return r.chromium.PDF(ctx)
	}
	return r.public.PDF(ctx)
}

func (r *Router) DescribeAction(ctx context.Context, ref string) (browser.ActionTarget, error) {
	if err := r.requireChromiumInteraction(); err != nil {
		return browser.ActionTarget{}, err
	}
	return r.chromium.DescribeAction(ctx, ref)
}

func (r *Router) CommitAction(ctx context.Context, target browser.ActionTarget) (browser.Snapshot, error) {
	if err := r.requireChromiumInteraction(); err != nil {
		return browser.Snapshot{}, err
	}
	return r.chromium.CommitAction(ctx, target)
}

func (r *Router) PrepareHumanTakeover(ctx context.Context) error {
	r.setSelection(browser.BackendModeChromium)
	if r.activeBackend() == BackendChromium {
		status, err := r.chromium.Status(ctx)
		if err != nil {
			return fmt.Errorf("read Chromium status before human login: %w", err)
		}
		r.pinChromiumURL(status.URL)
		return nil
	}
	snapshot, err := r.handoffCurrentToChromium(ctx)
	if err != nil {
		return err
	}
	r.pinChromiumURL(snapshot.URL)
	return nil
}

// CompleteHumanTakeover records the post-login host. Redirect-based SSO can
// finish on a different host than the page where takeover began, so both are
// kept on Chromium for the remainder of the browser session.
func (r *Router) CompleteHumanTakeover(ctx context.Context) error {
	status, err := r.chromium.Status(ctx)
	if err != nil {
		return fmt.Errorf("read Chromium status after human login: %w", err)
	}
	r.pinChromiumURL(status.URL)
	r.setSelection(browser.BackendModeChromium)
	r.setActive(BackendChromium, obscura.Snapshot{}, browser.Metadata{})
	return nil
}

func (r *Router) Close() error {
	var errs []error
	if r.public != nil {
		if err := r.public.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := r.chromium.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (r *Router) requiresChromium(rawURL string) bool {
	host := normalizedHost(rawURL)
	if host == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, domain := range r.alwaysChromium {
		if hostMatchesDomain(host, domain) {
			return true
		}
	}
	for domain := range r.pinnedChromiumHosts {
		if hostMatchesDomain(host, domain) {
			return true
		}
	}
	return false
}

func (r *Router) requireChromiumInteraction() error {
	if r.activeBackend() != BackendChromium {
		return errors.New("the current page is in the read-only Obscura backend; call browser_select_backend with backend ch/chromium before interacting; request human login only when authentication is actually required")
	}
	return nil
}

func (r *Router) handoffCurrentToChromium(ctx context.Context) (browser.Snapshot, error) {
	r.mu.Lock()
	targetURL := r.publicSnapshot.URL
	r.mu.Unlock()
	if targetURL == "" {
		return browser.Snapshot{}, errors.New("Obscura has no current URL to hand off")
	}
	snapshot, err := r.chromium.Open(ctx, targetURL)
	if err != nil {
		return browser.Snapshot{}, fmt.Errorf("hand off Obscura page to Chromium: %w", err)
	}
	r.setActive(BackendChromium, obscura.Snapshot{}, browser.Metadata{})
	return snapshot, nil
}

func normalizedHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
}

func hostMatchesDomain(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func (r *Router) pinChromiumURL(rawURL string) {
	host := normalizedHost(rawURL)
	if host == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pinnedChromiumHosts[host] = struct{}{}
}

func (r *Router) activeBackend() Backend {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

func (r *Router) selectedMode() browser.BackendMode {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.selection
}

func (r *Router) setSelection(mode browser.BackendMode) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.selection = mode
}

func (r *Router) setActive(active Backend, snapshot obscura.Snapshot, metadata browser.Metadata) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = active
	r.publicSnapshot = snapshot
	r.publicMetadata = metadata
}

func (r *Router) convertPublic(snapshot obscura.Snapshot) browser.Snapshot {
	r.mu.Lock()
	r.generation++
	generation := r.generation
	metadata := r.publicMetadata
	r.mu.Unlock()
	return browser.Snapshot{
		URL:        snapshot.URL,
		Title:      snapshot.Title,
		Text:       snapshot.Text,
		Elements:   []browser.Element{},
		Generation: generation,
		Backend:    string(BackendObscura),
		Metadata:   metadata,
	}
}

func (r *Router) currentPublicMetadata(rawURL string) browser.Metadata {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.publicSnapshot.URL != rawURL {
		return browser.Metadata{}
	}
	return r.publicMetadata
}

func (r *Router) fetchMetadata(ctx context.Context, rawURL string) browser.Metadata {
	if r.metadata == nil || strings.TrimSpace(rawURL) == "" {
		return browser.Metadata{}
	}
	metadata, err := r.metadata.Fetch(ctx, rawURL)
	if err != nil {
		if r.logger != nil {
			r.logger.Debug("page metadata unavailable", "url", rawURL, "error", err)
		}
		return browser.Metadata{}
	}
	return metadata
}

func (r *Router) publicAttemptAllowed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if r.circuitOpenUntil.IsZero() {
		return true
	}
	if now.Before(r.circuitOpenUntil) {
		return false
	}
	if r.halfOpenProbe {
		return false
	}
	r.halfOpenProbe = true
	return true
}

func (r *Router) recordPublicSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.consecutiveFailures = 0
	r.circuitOpenUntil = time.Time{}
	r.halfOpenProbe = false
}

func (r *Router) recordPublicFailure() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.consecutiveFailures++
	if r.halfOpenProbe || r.consecutiveFailures >= r.failureThreshold {
		r.circuitOpenUntil = r.now().Add(r.circuitCooldown)
	}
	r.halfOpenProbe = false
}

func (r *Router) routingStatus() *browser.RoutingStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	state := "closed"
	var retryAt *time.Time
	if !r.circuitOpenUntil.IsZero() {
		now := r.now()
		if now.Before(r.circuitOpenUntil) {
			state = "open"
			value := r.circuitOpenUntil
			retryAt = &value
		} else {
			state = "half_open"
		}
	}
	return &browser.RoutingStatus{
		PublicBackend:       string(BackendObscura),
		Mode:                string(r.selection),
		PinnedChromiumHosts: len(r.pinnedChromiumHosts),
		CircuitState:        state,
		ConsecutiveFailure:  r.consecutiveFailures,
		RetryAt:             retryAt,
	}
}

func (r *Router) fallbackCurrentToChromium(ctx context.Context, publicErr error) (browser.Snapshot, error) {
	r.mu.Lock()
	targetURL := r.publicSnapshot.URL
	r.mu.Unlock()
	if targetURL == "" {
		return browser.Snapshot{}, publicErr
	}
	if r.logger != nil {
		r.logger.Warn("Obscura read failed; falling back to Chromium", "url", targetURL, "error", publicErr)
	}
	snapshot, err := r.chromium.Open(ctx, targetURL)
	if err != nil {
		return browser.Snapshot{}, fmt.Errorf("Obscura failed (%v) and Chromium fallback failed: %w", publicErr, err)
	}
	r.setActive(BackendChromium, obscura.Snapshot{}, browser.Metadata{})
	return snapshot, nil
}
