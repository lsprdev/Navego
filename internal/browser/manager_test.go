package browser

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIsSensitiveActionLogout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		role string
		name string
		want bool
	}{
		{role: "menuitem", name: "Sair de @venn_mak", want: true},
		{role: "button", name: "Log out @venn_mak", want: true},
		{role: "link", name: "Sign out", want: true},
		{role: "button", name: "Logout", want: true},
		{role: "button", name: "Abrir menu da conta", want: false},
		{role: "textbox", name: "Sair", want: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.role+"/"+test.name, func(t *testing.T) {
			t.Parallel()
			if got := isSensitiveAction(test.role, test.name); got != test.want {
				t.Fatalf("isSensitiveAction(%q, %q) = %v; want %v", test.role, test.name, got, test.want)
			}
		})
	}
}

func TestRawApprovalURLsAreNotSerialized(t *testing.T) {
	values := []any{
		ActionTarget{URL: "https://example.com/[saved credential]", RawURL: "https://example.com/?user=private-value"},
		SavedLoginTarget{URL: "https://example.com/[saved credential]", RawURL: "https://example.com/?user=private-value"},
	}
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "private-value") {
			t.Fatalf("raw URL leaked from %T: %s", value, encoded)
		}
	}
}

func TestSavedCredentialsAreRedactedFromSnapshots(t *testing.T) {
	manager := &Manager{}
	manager.protectValueLocked([]byte("owner@example.com"))
	manager.protectValueLocked([]byte("test-password"))
	snapshot := Snapshot{
		URL:   "https://example.com/?account=owner@example.com",
		Title: "Welcome owner@example.com",
		Text:  "owner@example.com test-password",
		Elements: []Element{{
			Ref: "g1e1", Role: "textbox", Name: "owner@example.com", Value: "owner@example.com",
		}},
	}
	manager.redactProtectedValuesLocked(&snapshot)
	combined := snapshot.URL + snapshot.Title + snapshot.Text + snapshot.Elements[0].Name + snapshot.Elements[0].Value
	if strings.Contains(combined, "owner@example.com") || strings.Contains(combined, "test-password") {
		t.Fatalf("saved credential was not redacted: %+v", snapshot)
	}
	if !snapshot.Elements[0].Secret || snapshot.Elements[0].Value != "" {
		t.Fatalf("credential-bearing element was not masked: %+v", snapshot.Elements[0])
	}
}
