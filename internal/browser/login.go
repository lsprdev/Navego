package browser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/lsprdev/Navego/internal/credentials"
)

func (m *Manager) DescribeSavedLogin(ctx context.Context, usernameRef, passwordRef, submitRef string) (SavedLoginTarget, error) {
	if usernameRef == passwordRef || usernameRef == submitRef || passwordRef == submitRef {
		return SavedLoginTarget{}, errors.New("username, password, and submit refs must be different")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	username, err := m.refLocked(usernameRef)
	if err != nil {
		return SavedLoginTarget{}, err
	}
	password, err := m.refLocked(passwordRef)
	if err != nil {
		return SavedLoginTarget{}, err
	}
	submit, err := m.refLocked(submitRef)
	if err != nil {
		return SavedLoginTarget{}, err
	}
	if username.element.Secret || !isEditableRole(username.element.Role) {
		return SavedLoginTarget{}, errors.New("username ref must be a non-secret editable field")
	}
	if !password.element.Secret || password.element.Role != "textbox" {
		return SavedLoginTarget{}, errors.New("password ref must be a password field")
	}
	if submit.element.Role != "button" {
		return SavedLoginTarget{}, errors.New("submit ref must be a button")
	}
	if err := m.ensureConnectedLocked(); err != nil {
		return SavedLoginTarget{}, err
	}
	op, cancel := m.operationContext(ctx, m.actionTimeout)
	defer cancel()
	var currentURL string
	if err := chromedp.Run(op, chromedp.Location(&currentURL)); err != nil {
		return SavedLoginTarget{}, fmt.Errorf("read saved-login URL: %w", err)
	}
	origin, err := credentials.OriginFromURL(currentURL)
	if err != nil {
		return SavedLoginTarget{}, err
	}
	return SavedLoginTarget{
		URL:          m.redactStringLocked(currentURL),
		RawURL:       currentURL,
		Origin:       origin,
		Generation:   m.generation,
		UsernameRef:  usernameRef,
		UsernameName: username.element.Name,
		PasswordRef:  passwordRef,
		PasswordName: password.element.Name,
		SubmitRef:    submitRef,
		SubmitName:   submit.element.Name,
	}, nil
}

func (m *Manager) CommitSavedLogin(ctx context.Context, target SavedLoginTarget, username, password []byte) (Snapshot, error) {
	if len(username) == 0 || len(password) == 0 {
		return Snapshot{}, errors.New("saved username and password must not be empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if target.Generation != m.generation {
		return Snapshot{}, errors.New("the page snapshot changed after saved login was prepared")
	}
	usernameInfo, err := m.refLocked(target.UsernameRef)
	if err != nil {
		return Snapshot{}, err
	}
	passwordInfo, err := m.refLocked(target.PasswordRef)
	if err != nil {
		return Snapshot{}, err
	}
	submitInfo, err := m.refLocked(target.SubmitRef)
	if err != nil {
		return Snapshot{}, err
	}
	if usernameInfo.element.Secret || !isEditableRole(usernameInfo.element.Role) || usernameInfo.element.Name != target.UsernameName {
		return Snapshot{}, errors.New("the username field changed after approval was prepared")
	}
	if !passwordInfo.element.Secret || passwordInfo.element.Name != target.PasswordName {
		return Snapshot{}, errors.New("the password field changed after approval was prepared")
	}
	if submitInfo.element.Role != "button" || submitInfo.element.Name != target.SubmitName {
		return Snapshot{}, errors.New("the login button changed after approval was prepared")
	}
	if err := m.ensureConnectedLocked(); err != nil {
		return Snapshot{}, err
	}
	op, cancel := m.operationContext(ctx, m.navigationTimeout)
	defer cancel()
	validationScript := fmt.Sprintf(`(() => {
		const normalize = value => String(value || "").replace(/\s+/g, " ").trim();
		const nameFor = el => {
			if (!el) return "";
			const labelledBy = normalize(el.getAttribute("aria-labelledby"));
			const labelledText = labelledBy ? labelledBy.split(/\s+/).map(id => document.getElementById(id)?.innerText || "").join(" ") : "";
			const labelText = el.labels && el.labels.length ? Array.from(el.labels).map(label => label.innerText || "").join(" ") : "";
			return normalize(el.getAttribute("aria-label") || labelledText || labelText || el.getAttribute("placeholder") || el.getAttribute("title") || el.getAttribute("alt") || el.innerText || el.getAttribute("name") || el.value).slice(0, 180);
		};
		const username = document.querySelector(%s);
		const password = document.querySelector(%s);
		const submit = document.querySelector(%s);
		return {
			url: location.href,
			usernameOK: username instanceof HTMLInputElement || username instanceof HTMLTextAreaElement,
			usernameName: nameFor(username),
			passwordOK: password instanceof HTMLInputElement && String(password.type).toLowerCase() === "password",
			passwordName: nameFor(password),
			submitOK: !!submit && !submit.disabled,
			submitName: nameFor(submit)
		};
	})()`, strconv.Quote(usernameInfo.selector), strconv.Quote(passwordInfo.selector), strconv.Quote(submitInfo.selector))
	var current struct {
		URL          string `json:"url"`
		UsernameOK   bool   `json:"usernameOK"`
		UsernameName string `json:"usernameName"`
		PasswordOK   bool   `json:"passwordOK"`
		PasswordName string `json:"passwordName"`
		SubmitOK     bool   `json:"submitOK"`
		SubmitName   string `json:"submitName"`
	}
	if err := chromedp.Run(op, chromedp.Evaluate(validationScript, &current)); err != nil {
		return Snapshot{}, fmt.Errorf("validate saved-login controls: %w", err)
	}
	expectedURL := target.RawURL
	if expectedURL == "" {
		expectedURL = target.URL
	}
	if current.URL != expectedURL {
		return Snapshot{}, errors.New("the page URL changed after saved login was prepared")
	}
	currentOrigin, err := credentials.OriginFromURL(current.URL)
	if err != nil || currentOrigin != target.Origin {
		return Snapshot{}, errors.New("the page origin changed after saved login was prepared")
	}
	if !current.UsernameOK || current.UsernameName != target.UsernameName || !current.PasswordOK || current.PasswordName != target.PasswordName || !current.SubmitOK || current.SubmitName != target.SubmitName {
		return Snapshot{}, errors.New("the login form changed after approval was prepared")
	}

	usernameCopy := append([]byte(nil), username...)
	passwordCopy := append([]byte(nil), password...)
	defer clear(usernameCopy)
	defer clear(passwordCopy)
	fill := func(selector string, value []byte) chromedp.ActionFunc {
		return func(ctx context.Context) error {
			if err := chromedp.Focus(selector, chromedp.ByQuery).Do(ctx); err != nil {
				return err
			}
			if err := chromedp.KeyEvent("a", chromedp.KeyModifiers(input.ModifierCtrl)).Do(ctx); err != nil {
				return err
			}
			if err := chromedp.KeyEvent(kb.Backspace).Do(ctx); err != nil {
				return err
			}
			return input.InsertText(string(value)).Do(ctx)
		}
	}
	if err := chromedp.Run(op,
		fill(usernameInfo.selector, usernameCopy),
		fill(passwordInfo.selector, passwordCopy),
	); err != nil {
		return Snapshot{}, fmt.Errorf("fill saved login: %w", err)
	}
	verificationScript := fmt.Sprintf(`(() => {
		const username = document.querySelector(%s);
		const password = document.querySelector(%s);
		return { username: String(username?.value || ""), passwordLength: String(password?.value || "").length };
	})()`, strconv.Quote(usernameInfo.selector), strconv.Quote(passwordInfo.selector))
	var filled struct {
		Username       string `json:"username"`
		PasswordLength int    `json:"passwordLength"`
	}
	if err := chromedp.Run(op, chromedp.Evaluate(verificationScript, &filled)); err != nil {
		return Snapshot{}, fmt.Errorf("verify saved login fields: %w", err)
	}
	if !bytes.Equal([]byte(filled.Username), usernameCopy) || filled.PasswordLength != len(passwordCopy) {
		return Snapshot{}, errors.New("saved login field verification failed")
	}
	filled.Username = ""
	// Keep exact credential material out of every later model-visible snapshot,
	// including failed-login pages that preserve or echo submitted values.
	m.protectValueLocked(usernameCopy)
	m.protectValueLocked(passwordCopy)
	if err := chromedp.Run(op, chromedp.Click(submitInfo.selector, chromedp.ByQuery), chromedp.Sleep(750*time.Millisecond)); err != nil {
		return Snapshot{}, fmt.Errorf("submit saved login: %w", err)
	}
	return m.snapshotLocked(op)
}
