package takeover

import "testing"

func TestNextAutomationCallClaimsControl(t *testing.T) {
	state := New()
	if err := state.RequireAutomation(); err != nil {
		t.Fatal(err)
	}
	state.Request("login")
	if state.Status().Phase != HumanActive {
		t.Fatalf("got %s, want %s", state.Status().Phase, HumanActive)
	}
	if err := state.RequireAutomation(); err != nil {
		t.Fatal(err)
	}
	if state.Status().Phase != AutomationActive {
		t.Fatalf("got %s, want %s", state.Status().Phase, AutomationActive)
	}
}

func TestResumeIsIdempotent(t *testing.T) {
	state := New()
	status, err := state.Resume()
	if err != nil || status.Phase != AutomationActive {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	state.Request("login")
	status, err = state.Resume()
	if err != nil || status.Phase != AutomationActive {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}
