package browser

import (
	"context"
	"strings"
	"testing"
)

func TestPressKeyRejectsArbitraryAndUnsafeSubmission(t *testing.T) {
	manager := &Manager{refs: map[string]refInfo{
		"field": {element: Element{Ref: "field", Role: "textbox", Name: "Message"}},
		"send":  {element: Element{Ref: "send", Role: "button", Name: "Send", Sensitive: true}},
	}}
	if _, err := manager.PressKey(context.Background(), "", "CTRL+L"); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("arbitrary shortcut was not rejected: %v", err)
	}
	if _, err := manager.PressKey(context.Background(), "field", "ENTER"); err == nil || !strings.Contains(err.Error(), "submit a form") {
		t.Fatalf("enter on editable field was not rejected: %v", err)
	}
	if _, err := manager.PressKey(context.Background(), "send", "SPACE"); err == nil || !strings.Contains(err.Error(), "prepare_action") {
		t.Fatalf("space on sensitive control was not rejected: %v", err)
	}
}

func TestInteractionInputsAreBoundedBeforeBrowserUse(t *testing.T) {
	manager := &Manager{}
	if _, err := manager.SelectOption(context.Background(), "missing", ""); err == nil {
		t.Fatal("empty option should fail before browser use")
	}
	if _, err := manager.Scroll(context.Background(), "sideways", 100, ""); err == nil {
		t.Fatal("invalid scroll direction should fail before browser use")
	}
	if _, err := manager.Scroll(context.Background(), "down", 2001, ""); err == nil {
		t.Fatal("oversized scroll should fail before browser use")
	}
}
