package browser

import (
	"testing"
)

func TestDiscoverInitialTargetPrefersExistingPage(t *testing.T) {
	id, err := chooseInitialTarget([]debuggerTarget{
		{ID: "blank", Type: "page", URL: "about:blank"},
		{ID: "existing", Type: "page", URL: "https://example.com", Title: "Example"},
	})
	if err != nil || id != "existing" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}
