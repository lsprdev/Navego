package browser

import (
	"strings"
	"testing"
)

func TestSavedLoginPageError(t *testing.T) {
	for _, test := range []struct {
		name, url, text, code string
	}{
		{"blocked", "chrome-error://chromewebdata/", "private-user ERR_BLOCKED_BY_CLIENT private-password", "ERR_BLOCKED_BY_CLIENT"},
		{"dns", "chrome-error://chromewebdata/", "ERR_NAME_NOT_RESOLVED", "ERR_NAME_NOT_RESOLVED"},
		{"generic", "chrome-error://chromewebdata/", "private page contents", "CHROME_ERROR_PAGE"},
		{"chrome network error", "chrome://network-error/-20", "", "CHROME_ERROR_PAGE"},
		{"portal", "https://example.com/portal", "Welcome", ""},
		{"login form", "https://example.com/login.do", "Wrong password", ""},
		{"mfa", "https://example.com/mfa", "Enter code", ""},
		{"regular page quoting error", "https://example.com/help", "ERR_BLOCKED_BY_CLIENT", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := SavedLoginPageError(Snapshot{URL: test.url, Text: test.text})
			if test.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.code) {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), test.url) {
				t.Fatalf("leaked page contents: %v", err)
			}
		})
	}
}
