package takeover

import "testing"

func TestTakeoverBlocksAutomationUntilResume(t *testing.T) {
	state := New()
	if err := state.RequireAutomation(); err != nil {
		t.Fatal(err)
	}
	state.Request("login")
	if err := state.RequireAutomation(); err != ErrHumanControlActive {
		t.Fatalf("got %v, want %v", err, ErrHumanControlActive)
	}
	if _, err := state.Resume(); err != nil {
		t.Fatal(err)
	}
	if err := state.RequireAutomation(); err != nil {
		t.Fatal(err)
	}
}

func TestResumeWithoutTakeoverFails(t *testing.T) {
	if _, err := New().Resume(); err == nil {
		t.Fatal("expected resume to fail")
	}
}
