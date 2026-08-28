package loginapproval

import (
	"testing"
	"time"

	"github.com/lsprdev/Navego/internal/browser"
	"github.com/lsprdev/Navego/internal/credentials"
)

func TestSavedLoginApprovalIsOriginBoundAndSingleUse(t *testing.T) {
	store := NewStore(time.Minute)
	account := credentials.Descriptor{ID: "college", Label: "College", Origin: "https://portal.example.com"}
	target := browser.SavedLoginTarget{Origin: account.Origin, UsernameRef: "g1e1", PasswordRef: "g1e2", SubmitRef: "g1e3"}
	prepared, err := store.Prepare(account, target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Take(prepared.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Take(prepared.ID); err == nil {
		t.Fatal("saved-login approval replay should fail")
	}
	target.Origin = "https://evil.example"
	if _, err := store.Prepare(account, target); err == nil {
		t.Fatal("mismatched origin should fail")
	}
}
