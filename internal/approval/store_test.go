package approval

import (
	"testing"
	"time"

	"github.com/lsprdev/Navego/internal/browser"
)

func TestApprovalIsSingleUse(t *testing.T) {
	store := NewStore(time.Minute)
	approval, err := store.Prepare("Publish a post", browser.ActionTarget{Ref: "g1e1", Name: "Post"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Take(approval.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Take(approval.ID); err == nil {
		t.Fatal("expected replay to fail")
	}
}

func TestApprovalExpires(t *testing.T) {
	store := NewStore(time.Minute)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	approval, err := store.Prepare("Publish", browser.ActionTarget{Ref: "g1e1"})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := store.Take(approval.ID); err == nil {
		t.Fatal("expected expired approval to fail")
	}
}
