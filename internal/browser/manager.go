package browser

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

const (
	refAttribute         = "data-navego-ref"
	requestPolicyTimeout = 3 * time.Second
	maxManagedTabs       = 12
)

type tabContext struct {
	context context.Context
	cancel  context.CancelFunc
}

type refInfo struct {
	element  Element
	selector string
}

type Manager struct {
	mu sync.Mutex

	parent            context.Context
	cdpEndpoint       string
	actionTimeout     time.Duration
	navigationTimeout time.Duration
	maxChars          int
	maxElements       int
	urlPolicy         *PublicURLPolicy

	allocatorContext context.Context
	allocatorCancel  context.CancelFunc
	browserContext   context.Context
	browserCancel    context.CancelFunc
	tabContexts      map[target.ID]tabContext
	activeTarget     target.ID
	generation       uint64
	refs             map[string]refInfo
}

func NewManager(
	parent context.Context,
	cdpEndpoint string,
	actionTimeout time.Duration,
	navigationTimeout time.Duration,
	maxChars int,
	maxElements int,
) *Manager {
	return &Manager{
		parent:            parent,
		cdpEndpoint:       cdpEndpoint,
		actionTimeout:     actionTimeout,
		navigationTimeout: navigationTimeout,
		maxChars:          maxChars,
		maxElements:       maxElements,
		urlPolicy:         NewPublicURLPolicy(),
		refs:              make(map[string]refInfo),
	}
}

func (m *Manager) Status(ctx context.Context) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureConnectedLocked(); err != nil {
		return Status{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	var url, title string
	if err := chromedp.Run(op, chromedp.Location(&url), chromedp.Title(&title)); err != nil {
		return Status{}, fmt.Errorf("read browser status: %w", err)
	}
	return Status{Connected: true, URL: url, Title: title, Backend: "chromium"}, nil
}

func (m *Manager) Open(ctx context.Context, rawURL string) (Snapshot, error) {
	policyContext, policyCancel := context.WithTimeout(ctx, requestPolicyTimeout)
	u, err := m.urlPolicy.Validate(policyContext, rawURL)
	policyCancel()
	if err != nil {
		return Snapshot{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureConnectedLocked(); err != nil {
		return Snapshot{}, err
	}
	op, cancel := m.operationContext(ctx, m.navigationTimeout)
	defer cancel()
	if err := chromedp.Run(op, chromedp.Navigate(u.String()), chromedp.Sleep(250*time.Millisecond)); err != nil {
		return Snapshot{}, fmt.Errorf("navigate to %s: %w", u.Redacted(), err)
	}
	return m.snapshotLocked(op)
}

func (m *Manager) Snapshot(ctx context.Context) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureConnectedLocked(); err != nil {
		return Snapshot{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	return m.snapshotLocked(op)
}

func (m *Manager) Find(ctx context.Context, query string, limit int) (FindResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureConnectedLocked(); err != nil {
		return FindResult{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	snapshot, err := m.snapshotLocked(op)
	if err != nil {
		return FindResult{}, err
	}
	return FindSnapshot(snapshot, query, limit)
}

func (m *Manager) Wait(ctx context.Context, condition WaitCondition) (Snapshot, error) {
	condition, err := NormalizeWaitCondition(condition)
	if err != nil {
		return Snapshot{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureConnectedLocked(); err != nil {
		return Snapshot{}, err
	}
	op, cancel := m.operationContext(ctx, condition.Timeout)
	defer cancel()
	query := condition.Text
	field := "text"
	valueExpression := `document.body ? document.body.innerText : ""`
	if condition.URLContains != "" {
		query = condition.URLContains
		field = "url"
		valueExpression = "location.href"
	}
	waitScript := fmt.Sprintf(`(() => {
		const expected = %s.toLocaleLowerCase();
		const value = %s;
		return String(value || "").toLocaleLowerCase().includes(expected);
	})()`, strconv.Quote(query), valueExpression)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		var matched bool
		if err := chromedp.Run(op, chromedp.Evaluate(waitScript, &matched)); err != nil {
			if op.Err() != nil {
				return Snapshot{}, fmt.Errorf("wait for %s containing %q: %w", field, query, op.Err())
			}
			return Snapshot{}, fmt.Errorf("check browser wait condition: %w", err)
		}
		if matched {
			return m.snapshotLocked(op)
		}
		select {
		case <-op.Done():
			return Snapshot{}, fmt.Errorf("wait for %s containing %q: %w", field, query, op.Err())
		case <-ticker.C:
		}
	}
}

func (m *Manager) ListTabs(ctx context.Context) (TabsResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureConnectedLocked(); err != nil {
		return TabsResult{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	return m.listTabsLocked(op)
}

func (m *Manager) NewTab(ctx context.Context, rawURL string) (Snapshot, error) {
	policyContext, policyCancel := context.WithTimeout(ctx, requestPolicyTimeout)
	u, err := m.urlPolicy.Validate(policyContext, rawURL)
	policyCancel()
	if err != nil {
		return Snapshot{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureConnectedLocked(); err != nil {
		return Snapshot{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	tabs, err := m.listTabsLocked(op)
	cancel()
	if err != nil {
		return Snapshot{}, err
	}
	if len(tabs.Tabs) >= maxManagedTabs {
		return Snapshot{}, fmt.Errorf("cannot open more than %d Chromium tabs", maxManagedTabs)
	}
	var targetID target.ID
	op, cancel = m.operationContext(ctx, m.actionTimeout)
	err = chromedp.Run(op, chromedp.ActionFunc(func(ctx context.Context) error {
		var createErr error
		targetID, createErr = target.CreateTarget("about:blank").WithBackground(false).Do(ctx)
		return createErr
	}))
	cancel()
	if err != nil {
		return Snapshot{}, fmt.Errorf("create Chromium tab: %w", err)
	}
	if err := m.activateTargetLocked(ctx, targetID); err != nil {
		m.closeTargetBestEffortLocked(ctx, targetID)
		return Snapshot{}, err
	}
	op, cancel = m.operationContext(ctx, m.navigationTimeout)
	defer cancel()
	if err := chromedp.Run(op, chromedp.Navigate(u.String()), chromedp.Sleep(250*time.Millisecond)); err != nil {
		return Snapshot{}, fmt.Errorf("navigate new tab to %s: %w", u.Redacted(), err)
	}
	return m.snapshotLocked(op)
}

func (m *Manager) SwitchTab(ctx context.Context, tabID string) (Snapshot, error) {
	tabID = strings.TrimSpace(tabID)
	if tabID == "" || len(tabID) > 128 {
		return Snapshot{}, errors.New("tab_id must be a non-empty Chromium target ID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureConnectedLocked(); err != nil {
		return Snapshot{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	tabs, err := m.listTabsLocked(op)
	cancel()
	if err != nil {
		return Snapshot{}, err
	}
	if !tabExists(tabs, tabID) {
		return Snapshot{}, fmt.Errorf("unknown Chromium tab %q; call browser_list_tabs again", tabID)
	}
	if err := m.activateTargetLocked(ctx, target.ID(tabID)); err != nil {
		return Snapshot{}, err
	}
	op, cancel = m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	return m.snapshotLocked(op)
}

func (m *Manager) CloseTab(ctx context.Context, tabID string) (Snapshot, error) {
	tabID = strings.TrimSpace(tabID)
	if tabID == "" || len(tabID) > 128 {
		return Snapshot{}, errors.New("tab_id must be a non-empty Chromium target ID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureConnectedLocked(); err != nil {
		return Snapshot{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	tabs, err := m.listTabsLocked(op)
	cancel()
	if err != nil {
		return Snapshot{}, err
	}
	if !tabExists(tabs, tabID) {
		return Snapshot{}, fmt.Errorf("unknown Chromium tab %q; call browser_list_tabs again", tabID)
	}
	if len(tabs.Tabs) == 1 {
		return Snapshot{}, errors.New("cannot close the last Chromium tab")
	}
	closingID := target.ID(tabID)
	if closingID == m.activeTarget {
		for _, candidate := range tabs.Tabs {
			if candidate.ID != tabID {
				if err := m.activateTargetLocked(ctx, target.ID(candidate.ID)); err != nil {
					return Snapshot{}, fmt.Errorf("activate replacement tab: %w", err)
				}
				break
			}
		}
	}
	if managed, exists := m.tabContexts[closingID]; exists {
		managed.cancel()
		delete(m.tabContexts, closingID)
	} else {
		closingContext, closingCancel := chromedp.NewContext(m.allocatorContext, chromedp.WithTargetID(closingID))
		stopRequest := context.AfterFunc(ctx, closingCancel)
		timeout := time.AfterFunc(m.actionTimeout, closingCancel)
		err = chromedp.Run(closingContext)
		stopRequest()
		timeout.Stop()
		closingCancel()
		if err != nil {
			return Snapshot{}, fmt.Errorf("attach Chromium tab %s for closing: %w", tabID, err)
		}
	}
	m.refs = make(map[string]refInfo)
	op, cancel = m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	return m.snapshotLocked(op)
}

func (m *Manager) Click(ctx context.Context, ref string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, err := m.refLocked(ref)
	if err != nil {
		return Snapshot{}, err
	}
	if info.element.Sensitive {
		return Snapshot{}, fmt.Errorf(
			"%s %q may cause an external effect; use browser_prepare_action and wait for explicit confirmation",
			info.element.Role,
			info.element.Name,
		)
	}
	return m.clickLocked(ctx, info)
}

func (m *Manager) Type(ctx context.Context, ref, text string, clear bool) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, err := m.refLocked(ref)
	if err != nil {
		return Snapshot{}, err
	}
	if info.element.Secret {
		return Snapshot{}, errors.New("typing into password or secret fields is blocked; request human login instead")
	}
	if err := m.ensureConnectedLocked(); err != nil {
		return Snapshot{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	activationScript := fmt.Sprintf(`(() => {
		const refAttribute = %s;
		const original = document.querySelector(%s);
		const active = document.activeElement;
		const editable = el => !!el && (el.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName) || ["textbox", "searchbox", "combobox"].includes(el.getAttribute("role")));
		const target = editable(active) ? active : original;
		if (!target) return { found: false, role: "", name: "", contentEditable: false };
		if (original && original !== target) original.removeAttribute(refAttribute);
		target.setAttribute(refAttribute, %s);
		const role = target.getAttribute("role") || (target.tagName === "SELECT" ? "combobox" : target.tagName === "INPUT" && target.type === "search" ? "searchbox" : ["INPUT", "TEXTAREA"].includes(target.tagName) || target.isContentEditable ? "textbox" : target.tagName.toLowerCase());
		const name = String(target.getAttribute("aria-label") || target.getAttribute("placeholder") || target.getAttribute("title") || target.getAttribute("name") || "").replace(/\s+/g, " ").trim().slice(0, 180);
		return { found: true, role, name, contentEditable: target.isContentEditable };
	})()`, strconv.Quote(refAttribute), strconv.Quote(info.selector), strconv.Quote(ref))
	var activated struct {
		Found           bool   `json:"found"`
		Role            string `json:"role"`
		Name            string `json:"name"`
		ContentEditable bool   `json:"contentEditable"`
	}
	if err := chromedp.Run(op,
		chromedp.Click(info.selector, chromedp.ByQuery),
		chromedp.Sleep(150*time.Millisecond),
		chromedp.Evaluate(activationScript, &activated),
	); err != nil {
		return Snapshot{}, fmt.Errorf("activate editable %s: %w", ref, err)
	}
	if !activated.Found || activated.Role != info.element.Role || (activated.Name != "" && info.element.Name != "" && activated.Name != info.element.Name) {
		return Snapshot{}, errors.New("the editable element changed while it was being activated; take a new snapshot")
	}

	var actual string
	readValueScript := fmt.Sprintf(`(() => {
		const el = document.querySelector(%s);
		if (!el) return "";
		return String(el.isContentEditable ? el.innerText : ("value" in el ? el.value : el.innerText) || "");
	})()`, strconv.Quote(info.selector))
	var actions []chromedp.Action
	if activated.ContentEditable {
		selectionScript := fmt.Sprintf(`(() => {
			const el = document.querySelector(%s);
			if (!el || !el.isContentEditable) return false;
			el.focus();
			if (%t) {
				const selection = window.getSelection();
				const range = document.createRange();
				range.selectNodeContents(el);
				selection.removeAllRanges();
				selection.addRange(range);
			}
			return true;
		})()`, strconv.Quote(info.selector), clear)
		var selected bool
		actions = append(actions,
			chromedp.Evaluate(selectionScript, &selected),
			chromedp.ActionFunc(func(ctx context.Context) error {
				if text == "" {
					return chromedp.KeyEvent(kb.Backspace).Do(ctx)
				}
				return input.InsertText(text).Do(ctx)
			}),
			chromedp.Sleep(200*time.Millisecond),
			chromedp.Evaluate(readValueScript, &actual),
		)
	} else {
		actions = append(actions,
			chromedp.Focus(info.selector, chromedp.ByQuery),
			chromedp.Sleep(100*time.Millisecond),
		)
		if clear && info.element.Value != "" {
			actions = append(actions, chromedp.KeyEvent("a", chromedp.KeyModifiers(input.ModifierCtrl)))
			if text == "" {
				actions = append(actions, chromedp.KeyEvent(kb.Backspace))
			}
			actions = append(actions, chromedp.Sleep(50*time.Millisecond))
		}
		actions = append(actions,
			chromedp.SendKeys(info.selector, text, chromedp.ByQuery),
			chromedp.Sleep(200*time.Millisecond),
			chromedp.Evaluate(readValueScript, &actual),
		)
	}
	if err := chromedp.Run(op, actions...); err != nil {
		return Snapshot{}, fmt.Errorf("type into %s: %w", ref, err)
	}
	expected := info.element.Value + text
	if clear {
		expected = text
	}
	if actual != expected {
		return Snapshot{}, fmt.Errorf("typing verification failed for %s: the field contains %q instead of %q", ref, actual, expected)
	}
	return m.snapshotLocked(op)
}

func (m *Manager) Screenshot(ctx context.Context, fullPage bool) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureConnectedLocked(); err != nil {
		return nil, "", err
	}
	op, cancel := m.operationContext(ctx, m.navigationTimeout)
	defer cancel()
	var data []byte
	var action chromedp.Action = chromedp.CaptureScreenshot(&data)
	if fullPage {
		action = chromedp.FullScreenshot(&data, 100)
	}
	if err := chromedp.Run(op, action); err != nil {
		return nil, "", fmt.Errorf("capture screenshot: %w", err)
	}
	return data, "image/png", nil
}

func (m *Manager) PDF(ctx context.Context) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureConnectedLocked(); err != nil {
		return nil, "", err
	}
	op, cancel := m.operationContext(ctx, m.navigationTimeout)
	defer cancel()
	var data []byte
	if err := chromedp.Run(op, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		data, _, err = page.PrintToPDF().WithPrintBackground(true).Do(ctx)
		return err
	})); err != nil {
		return nil, "", fmt.Errorf("export Chromium PDF: %w", err)
	}
	return data, "application/pdf", nil
}

func (m *Manager) DescribeAction(ctx context.Context, ref string) (ActionTarget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, err := m.refLocked(ref)
	if err != nil {
		return ActionTarget{}, err
	}
	if err := m.ensureConnectedLocked(); err != nil {
		return ActionTarget{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	var url string
	if err := chromedp.Run(op, chromedp.Location(&url)); err != nil {
		return ActionTarget{}, fmt.Errorf("read action URL: %w", err)
	}
	fields := make(map[string]string)
	fieldRefs := make(map[string]string)
	for _, candidate := range m.refs {
		if candidate.element.Secret || candidate.element.Value == "" || !isEditableRole(candidate.element.Role) {
			continue
		}
		name := candidate.element.Name
		if name == "" {
			name = candidate.element.Ref
		}
		if _, exists := fields[name]; exists {
			name = name + " (" + candidate.element.Ref + ")"
		}
		fields[name] = candidate.element.Value
		fieldRefs[candidate.element.Ref] = candidate.element.Value
	}
	return ActionTarget{
		Ref:        ref,
		Role:       info.element.Role,
		Name:       info.element.Name,
		URL:        url,
		Generation: m.generation,
		Fields:     fields,
		FieldRefs:  fieldRefs,
	}, nil
}

func (m *Manager) CommitAction(ctx context.Context, target ActionTarget) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if target.Generation != m.generation {
		return Snapshot{}, errors.New("the page snapshot changed after approval was prepared")
	}
	info, err := m.refLocked(target.Ref)
	if err != nil {
		return Snapshot{}, err
	}
	if info.element.Role != target.Role || info.element.Name != target.Name {
		return Snapshot{}, errors.New("the target element changed after approval was prepared")
	}
	if err := m.ensureConnectedLocked(); err != nil {
		return Snapshot{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	validationScript := fmt.Sprintf(`(() => {
		const normalize = value => String(value || "").replace(/\s+/g, " ").trim();
		const target = document.querySelector(%s);
		if (!target) return { found: false, url: location.href, name: "", fields: {} };
		const name = normalize(
			target.getAttribute("aria-label") || target.getAttribute("placeholder") || target.getAttribute("title") ||
			target.getAttribute("alt") || target.innerText || target.getAttribute("name") || target.value
		).slice(0, 180);
		const fields = {};
		for (const el of document.querySelectorAll("[%s]")) {
			const ref = el.getAttribute(%s);
			const secret = el.tagName.toLowerCase() === "input" && (el.getAttribute("type") || "").toLowerCase() === "password";
			if (!secret && ("value" in el || el.isContentEditable)) {
				fields[ref] = normalize(el.isContentEditable ? el.innerText : el.value).slice(0, 240);
			}
		}
		return { found: true, url: location.href, name, fields };
	})()`, strconv.Quote(info.selector), refAttribute, strconv.Quote(refAttribute))
	var current struct {
		Found  bool              `json:"found"`
		URL    string            `json:"url"`
		Name   string            `json:"name"`
		Fields map[string]string `json:"fields"`
	}
	if err := chromedp.Run(op, chromedp.Evaluate(validationScript, &current)); err != nil {
		return Snapshot{}, fmt.Errorf("validate prepared action: %w", err)
	}
	if !current.Found {
		return Snapshot{}, errors.New("the target element disappeared after approval was prepared")
	}
	if current.URL != target.URL {
		return Snapshot{}, errors.New("the page URL changed after approval was prepared")
	}
	if target.Name != "" && current.Name != target.Name {
		return Snapshot{}, errors.New("the target label changed after approval was prepared")
	}
	for ref, value := range target.FieldRefs {
		if current.Fields[ref] != value {
			return Snapshot{}, errors.New("page input changed after approval was prepared")
		}
	}
	if err := chromedp.Run(op, chromedp.Click(info.selector, chromedp.ByQuery), chromedp.Sleep(500*time.Millisecond)); err != nil {
		return Snapshot{}, fmt.Errorf("commit action on %s: %w", target.Ref, err)
	}
	return m.snapshotLocked(op)
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Chromium is a separate persistent service. Do not cancel chromedp target
	// contexts during a normal gateway shutdown because chromedp cancellation
	// closes the attached tabs. The process exit releases the CDP connection.
	m.browserContext = nil
	m.browserCancel = nil
	m.allocatorContext = nil
	m.allocatorCancel = nil
	m.tabContexts = nil
	m.activeTarget = ""
	m.refs = make(map[string]refInfo)
	return nil
}

func (m *Manager) clickLocked(ctx context.Context, info refInfo) (Snapshot, error) {
	if err := m.ensureConnectedLocked(); err != nil {
		return Snapshot{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	if err := chromedp.Run(op, chromedp.Click(info.selector, chromedp.ByQuery), chromedp.Sleep(350*time.Millisecond)); err != nil {
		return Snapshot{}, fmt.Errorf("click %s: %w", info.element.Ref, err)
	}
	return m.snapshotLocked(op)
}

func (m *Manager) snapshotLocked(ctx context.Context) (Snapshot, error) {
	m.generation++
	prefix := fmt.Sprintf("g%de", m.generation)
	script := strings.NewReplacer(
		"__PREFIX__", strconv.Quote(prefix),
		"__MAX_ELEMENTS__", strconv.Itoa(m.maxElements),
		"__MAX_CHARS__", strconv.Itoa(m.maxChars),
	).Replace(snapshotScript)
	var snapshot Snapshot
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &snapshot)); err != nil {
		return Snapshot{}, fmt.Errorf("capture page snapshot: %w", err)
	}
	snapshot.Generation = m.generation
	snapshot.Backend = "chromium"
	m.refs = make(map[string]refInfo, len(snapshot.Elements))
	for index := range snapshot.Elements {
		element := &snapshot.Elements[index]
		element.Sensitive = isSensitiveAction(element.Role, element.Name)
		m.refs[element.Ref] = refInfo{
			element:  *element,
			selector: fmt.Sprintf("[%s=%s]", refAttribute, strconv.Quote(element.Ref)),
		}
	}
	return snapshot, nil
}

func (m *Manager) refLocked(ref string) (refInfo, error) {
	info, ok := m.refs[ref]
	if !ok {
		return refInfo{}, fmt.Errorf("unknown or stale element ref %q; call browser_snapshot again", ref)
	}
	return info, nil
}

func (m *Manager) ensureConnectedLocked() error {
	if m.browserContext != nil && m.browserContext.Err() == nil {
		return nil
	}
	m.resetLocked()
	allocatorParent := context.WithoutCancel(m.parent)
	m.allocatorContext, m.allocatorCancel = chromedp.NewRemoteAllocator(allocatorParent, m.cdpEndpoint)
	initialTarget, discoveryErr := discoverInitialTarget(m.parent, m.cdpEndpoint)
	if discoveryErr == nil {
		m.browserContext, m.browserCancel = chromedp.NewContext(m.allocatorContext, chromedp.WithTargetID(initialTarget))
	} else {
		m.browserContext, m.browserCancel = chromedp.NewContext(m.allocatorContext)
	}
	if err := chromedp.Run(m.browserContext); err != nil {
		m.resetLocked()
		return fmt.Errorf("connect to Chromium at %s: %w", m.cdpEndpoint, err)
	}
	if err := m.enableRequestPolicyLocked(m.browserContext); err != nil {
		m.resetLocked()
		return err
	}
	current := chromedp.FromContext(m.browserContext)
	if current == nil || current.Target == nil {
		m.resetLocked()
		return errors.New("Chromium connection did not expose an active target")
	}
	m.activeTarget = current.Target.TargetID
	m.tabContexts = map[target.ID]tabContext{
		m.activeTarget: {context: m.browserContext, cancel: m.browserCancel},
	}
	m.refs = make(map[string]refInfo)
	return nil
}

func (m *Manager) enableRequestPolicyLocked(browserContext context.Context) error {
	chromedp.ListenTarget(browserContext, func(event any) {
		paused, ok := event.(*fetch.EventRequestPaused)
		if !ok || paused.Request == nil {
			return
		}
		go m.handlePausedRequest(browserContext, paused)
	})
	if err := chromedp.Run(browserContext, chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.Enable().WithPatterns([]*fetch.RequestPattern{{
			URLPattern:   "*",
			RequestStage: fetch.RequestStageRequest,
		}}).Do(ctx)
	})); err != nil {
		return fmt.Errorf("enable browser request policy: %w", err)
	}
	return nil
}

func (m *Manager) listTabsLocked(ctx context.Context) (TabsResult, error) {
	targets, err := chromedp.Targets(ctx)
	if err != nil {
		return TabsResult{}, fmt.Errorf("list Chromium tabs: %w", err)
	}
	result := TabsResult{Tabs: []Tab{}}
	for _, info := range targets {
		if info == nil || info.Type != "page" {
			continue
		}
		result.Tabs = append(result.Tabs, Tab{
			ID:     string(info.TargetID),
			URL:    info.URL,
			Title:  info.Title,
			Active: info.TargetID == m.activeTarget,
		})
		if len(result.Tabs) >= 50 {
			break
		}
	}
	return result, nil
}

func (m *Manager) activateTargetLocked(request context.Context, targetID target.ID) error {
	if targetID == "" {
		return errors.New("Chromium target ID must not be empty")
	}
	if managed, exists := m.tabContexts[targetID]; exists && managed.context.Err() == nil {
		m.browserContext = managed.context
		m.browserCancel = managed.cancel
		m.activeTarget = targetID
		m.refs = make(map[string]refInfo)
		return nil
	}
	tabCtx, tabCancel := chromedp.NewContext(m.allocatorContext, chromedp.WithTargetID(targetID))
	stopRequest := context.AfterFunc(request, tabCancel)
	timeout := time.AfterFunc(m.actionTimeout, tabCancel)
	err := chromedp.Run(tabCtx)
	stopRequest()
	timeout.Stop()
	if err != nil {
		tabCancel()
		return fmt.Errorf("attach Chromium tab %s: %w", targetID, err)
	}
	if err := m.enableRequestPolicyLocked(tabCtx); err != nil {
		tabCancel()
		return err
	}
	m.tabContexts[targetID] = tabContext{context: tabCtx, cancel: tabCancel}
	m.browserContext = tabCtx
	m.browserCancel = tabCancel
	m.activeTarget = targetID
	m.refs = make(map[string]refInfo)
	return nil
}

func (m *Manager) closeTargetBestEffortLocked(request context.Context, targetID target.ID) {
	if m.allocatorContext == nil || m.allocatorContext.Err() != nil {
		return
	}
	closingContext, closingCancel := chromedp.NewContext(m.allocatorContext, chromedp.WithTargetID(targetID))
	stopRequest := context.AfterFunc(request, closingCancel)
	timeout := time.AfterFunc(m.actionTimeout, closingCancel)
	_ = chromedp.Run(closingContext)
	stopRequest()
	timeout.Stop()
	closingCancel()
}

func tabExists(tabs TabsResult, tabID string) bool {
	for _, tab := range tabs.Tabs {
		if tab.ID == tabID {
			return true
		}
	}
	return false
}

func (m *Manager) handlePausedRequest(browserContext context.Context, paused *fetch.EventRequestPaused) {
	policyContext, policyCancel := context.WithTimeout(browserContext, requestPolicyTimeout)
	_, policyErr := m.urlPolicy.Validate(policyContext, paused.Request.URL)
	policyCancel()

	chromedpContext := chromedp.FromContext(browserContext)
	if chromedpContext == nil || chromedpContext.Target == nil || browserContext.Err() != nil {
		return
	}
	executorContext := cdp.WithExecutor(browserContext, chromedpContext.Target)
	if policyErr != nil {
		_ = fetch.FailRequest(paused.RequestID, network.ErrorReasonBlockedByClient).Do(executorContext)
		return
	}
	_ = fetch.ContinueRequest(paused.RequestID).Do(executorContext)
}

func (m *Manager) operationContext(request context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(m.browserContext, timeout)
	stop := context.AfterFunc(request, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

func (m *Manager) resetLocked() {
	if len(m.tabContexts) > 0 {
		for id, managed := range m.tabContexts {
			managed.cancel()
			delete(m.tabContexts, id)
		}
	} else if m.browserCancel != nil {
		m.browserCancel()
	}
	if m.allocatorCancel != nil {
		m.allocatorCancel()
	}
	m.browserContext = nil
	m.browserCancel = nil
	m.tabContexts = nil
	m.activeTarget = ""
	m.allocatorContext = nil
	m.allocatorCancel = nil
	m.refs = make(map[string]refInfo)
}

func isEditableRole(role string) bool {
	switch role {
	case "textbox", "searchbox", "combobox":
		return true
	default:
		return false
	}
}

func isSensitiveAction(role, name string) bool {
	if role != "button" && role != "link" && role != "menuitem" {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(name))
	for _, word := range []string{
		"post", "publish", "send", "submit", "buy", "purchase", "order", "pay", "delete", "remove", "confirm",
		"log out", "logout", "sign out",
		"postar", "publicar", "enviar", "comprar", "pagar", "excluir", "remover", "confirmar", "tweetar",
		"sair", "encerrar sessão", "terminar sessão",
	} {
		if normalized == word || strings.HasPrefix(normalized, word+" ") {
			return true
		}
	}
	return false
}

const snapshotScript = `(() => {
	const refAttribute = "data-navego-ref";
	const prefix = __PREFIX__;
	const maxElements = __MAX_ELEMENTS__;
	const maxChars = __MAX_CHARS__;
	for (const el of document.querySelectorAll("[" + refAttribute + "]")) {
		el.removeAttribute(refAttribute);
	}
	const normalize = value => String(value || "").replace(/\s+/g, " ").trim();
	const visible = el => {
		const style = getComputedStyle(el);
		const rect = el.getBoundingClientRect();
		return style.display !== "none" && style.visibility !== "hidden" && Number(style.opacity || 1) !== 0 && rect.width > 0 && rect.height > 0;
	};
	const roleFor = el => {
		const explicit = normalize(el.getAttribute("role"));
		if (explicit) return explicit;
		const tag = el.tagName.toLowerCase();
		if (tag === "a") return "link";
		if (tag === "button") return "button";
		if (tag === "textarea") return "textbox";
		if (tag === "select") return "combobox";
		if (tag === "input") {
			const type = (el.getAttribute("type") || "text").toLowerCase();
			if (type === "checkbox") return "checkbox";
			if (type === "radio") return "radio";
			if (type === "search") return "searchbox";
			if (["button", "submit", "reset"].includes(type)) return "button";
			return "textbox";
		}
		if (el.isContentEditable) return "textbox";
		return tag;
	};
	const nameFor = el => {
		const labelledBy = normalize(el.getAttribute("aria-labelledby"));
		let labelledText = "";
		if (labelledBy) {
			labelledText = labelledBy.split(/\s+/).map(id => document.getElementById(id)?.innerText || "").join(" ");
		}
		let labelText = "";
		if (el.labels && el.labels.length) labelText = Array.from(el.labels).map(label => label.innerText || "").join(" ");
		return normalize(
			el.getAttribute("aria-label") || labelledText || labelText || el.getAttribute("placeholder") ||
			el.getAttribute("title") || el.getAttribute("alt") || el.innerText || el.getAttribute("name") || el.value
		).slice(0, 180);
	};
	const candidates = document.querySelectorAll(
		"a[href],button,input,textarea,select,[role],[contenteditable='true'],[tabindex]:not([tabindex='-1'])"
	);
	const elements = [];
	for (const el of candidates) {
		if (elements.length >= maxElements || !visible(el) || el.disabled || el.getAttribute("aria-hidden") === "true") continue;
		const ref = prefix + (elements.length + 1);
		el.setAttribute(refAttribute, ref);
		const role = roleFor(el);
		const secret = el.tagName.toLowerCase() === "input" && (el.getAttribute("type") || "").toLowerCase() === "password";
		let value = "";
		if (!secret && ("value" in el || el.isContentEditable)) value = normalize(el.isContentEditable ? el.innerText : el.value).slice(0, 240);
		elements.push({ ref, role, name: nameFor(el), value, secret });
	}
	const text = normalize(document.body ? document.body.innerText : "").slice(0, maxChars);
	const meta = key => normalize(document.querySelector('meta[property="' + key + '"],meta[name="' + key + '"]')?.content || "");
	const publicURL = raw => {
		if (!raw) return "";
		try {
			const parsed = new URL(raw, location.href);
			return ["http:", "https:"].includes(parsed.protocol) ? parsed.href : "";
		} catch (_) {
			return "";
		}
	};
	const metadata = {
		description: (meta("og:description") || meta("description")).slice(0, 500),
		image_url: publicURL(meta("og:image") || meta("twitter:image")),
		image_alt: meta("og:image:alt").slice(0, 300),
		site_name: meta("og:site_name").slice(0, 200),
		type: meta("og:type").slice(0, 100),
		article_section: meta("article:section").slice(0, 200),
	};
	return { url: location.href, title: document.title, text, elements, metadata };
})()`
