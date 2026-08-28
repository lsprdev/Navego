package browser

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

var allowedKeys = map[string]string{
	"TAB":        kb.Tab,
	"ENTER":      kb.Enter,
	"ESCAPE":     kb.Escape,
	"SPACE":      " ",
	"ARROWUP":    kb.ArrowUp,
	"ARROWDOWN":  kb.ArrowDown,
	"ARROWLEFT":  kb.ArrowLeft,
	"ARROWRIGHT": kb.ArrowRight,
	"HOME":       kb.Home,
	"END":        kb.End,
	"PAGEUP":     kb.PageUp,
	"PAGEDOWN":   kb.PageDown,
}

func (m *Manager) Hover(ctx context.Context, ref string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, err := m.refLocked(ref)
	if err != nil {
		return Snapshot{}, err
	}
	if err := m.ensureConnectedLocked(); err != nil {
		return Snapshot{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	positionScript := fmt.Sprintf(`(() => {
		const el = document.querySelector(%s);
		if (!el) return { found: false, x: 0, y: 0 };
		el.scrollIntoView({ block: "center", inline: "center" });
		const rect = el.getBoundingClientRect();
		if (rect.width <= 0 || rect.height <= 0) return { found: false, x: 0, y: 0 };
		return { found: true, x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
	})()`, strconv.Quote(info.selector))
	var position struct {
		Found bool    `json:"found"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
	}
	if err := chromedp.Run(op, chromedp.Evaluate(positionScript, &position)); err != nil {
		return Snapshot{}, fmt.Errorf("locate element %s for hover: %w", ref, err)
	}
	if !position.Found {
		return Snapshot{}, errors.New("the element is no longer visible; take a new snapshot")
	}
	if err := chromedp.Run(op,
		chromedp.MouseEvent(input.MouseMoved, position.X, position.Y),
		chromedp.Sleep(300*time.Millisecond),
	); err != nil {
		return Snapshot{}, fmt.Errorf("hover element %s: %w", ref, err)
	}
	return m.snapshotLocked(op)
}

func (m *Manager) PressKey(ctx context.Context, ref, key string) (Snapshot, error) {
	keyName := strings.ToUpper(strings.TrimSpace(key))
	keyValue, ok := allowedKeys[keyName]
	if !ok {
		return Snapshot{}, fmt.Errorf("key %q is not allowed; use TAB, ENTER, ESCAPE, SPACE, arrows, HOME, END, PAGEUP, or PAGEDOWN", key)
	}
	ref = strings.TrimSpace(ref)
	m.mu.Lock()
	defer m.mu.Unlock()
	var info refInfo
	if ref != "" {
		var err error
		info, err = m.refLocked(ref)
		if err != nil {
			return Snapshot{}, err
		}
	}
	if keyName == "ENTER" || keyName == "SPACE" {
		if ref == "" {
			return Snapshot{}, fmt.Errorf("%s requires an explicit element ref", keyName)
		}
		if info.element.Sensitive {
			return Snapshot{}, fmt.Errorf("%s on %s %q may cause an external effect; use browser_prepare_action instead", keyName, info.element.Role, info.element.Name)
		}
		if isEditableRole(info.element.Role) {
			return Snapshot{}, fmt.Errorf("%s on an editable field may submit a form; use a button, menuitem, option, or link ref", keyName)
		}
	}
	if err := m.ensureConnectedLocked(); err != nil {
		return Snapshot{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	actions := make([]chromedp.Action, 0, 3)
	if ref != "" {
		actions = append(actions, chromedp.Focus(info.selector, chromedp.ByQuery))
	}
	actions = append(actions, chromedp.KeyEvent(keyValue), chromedp.Sleep(250*time.Millisecond))
	if err := chromedp.Run(op, actions...); err != nil {
		return Snapshot{}, fmt.Errorf("press key %s: %w", keyName, err)
	}
	return m.snapshotLocked(op)
}

func (m *Manager) SelectOption(ctx context.Context, ref, option string) (Snapshot, error) {
	option = strings.TrimSpace(option)
	if option == "" || len(option) > 500 {
		return Snapshot{}, errors.New("option must be a non-empty value or visible label up to 500 characters")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	info, err := m.refLocked(ref)
	if err != nil {
		return Snapshot{}, err
	}
	if info.element.Role != "combobox" {
		return Snapshot{}, fmt.Errorf("element %s is %s, not a native combobox", ref, info.element.Role)
	}
	if err := m.ensureConnectedLocked(); err != nil {
		return Snapshot{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	selectScript := fmt.Sprintf(`(() => {
		const el = document.querySelector(%s);
		if (!(el instanceof HTMLSelectElement)) return { found: false, selected: "" };
		const wanted = %s;
		const candidate = Array.from(el.options).find(option => option.value === wanted || option.text.trim() === wanted);
		if (!candidate || candidate.disabled) return { found: true, selected: "" };
		el.value = candidate.value;
		el.dispatchEvent(new Event("input", { bubbles: true }));
		el.dispatchEvent(new Event("change", { bubbles: true }));
		return { found: true, selected: candidate.text.trim() || candidate.value };
	})()`, strconv.Quote(info.selector), strconv.Quote(option))
	var selected struct {
		Found    bool   `json:"found"`
		Selected string `json:"selected"`
	}
	if err := chromedp.Run(op, chromedp.Evaluate(selectScript, &selected), chromedp.Sleep(250*time.Millisecond)); err != nil {
		return Snapshot{}, fmt.Errorf("select option on %s: %w", ref, err)
	}
	if !selected.Found {
		return Snapshot{}, errors.New("this combobox is custom; use click plus allowed arrow/enter keys")
	}
	if selected.Selected == "" {
		return Snapshot{}, fmt.Errorf("option %q was not found or is disabled", option)
	}
	return m.snapshotLocked(op)
}

func (m *Manager) Scroll(ctx context.Context, direction string, amount int, ref string) (Snapshot, error) {
	direction = strings.ToLower(strings.TrimSpace(direction))
	ref = strings.TrimSpace(ref)
	if ref == "" && direction != "up" && direction != "down" {
		return Snapshot{}, errors.New("direction must be up or down when ref is empty")
	}
	if amount == 0 {
		amount = 700
	}
	if amount < 1 || amount > 2000 {
		return Snapshot{}, errors.New("amount must be between 1 and 2000 pixels")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var info refInfo
	if ref != "" {
		var err error
		info, err = m.refLocked(ref)
		if err != nil {
			return Snapshot{}, err
		}
	}
	if err := m.ensureConnectedLocked(); err != nil {
		return Snapshot{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	var script string
	if ref != "" {
		script = fmt.Sprintf(`(() => {
			const el = document.querySelector(%s);
			if (!el) return false;
			el.scrollIntoView({ block: "center", inline: "nearest" });
			return true;
		})()`, strconv.Quote(info.selector))
	} else {
		delta := amount
		if direction == "up" {
			delta = -amount
		}
		script = fmt.Sprintf(`(() => { window.scrollBy({ top: %d, behavior: "instant" }); return true; })()`, delta)
	}
	var scrolled bool
	if err := chromedp.Run(op, chromedp.Evaluate(script, &scrolled), chromedp.Sleep(250*time.Millisecond)); err != nil {
		return Snapshot{}, fmt.Errorf("scroll page: %w", err)
	}
	if !scrolled {
		return Snapshot{}, errors.New("the target element disappeared; take a new snapshot")
	}
	return m.snapshotLocked(op)
}
