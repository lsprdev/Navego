package takeover

import (
	"sync"
	"time"
)

type Phase string

const (
	AutomationActive Phase = "automation_active"
	HumanActive      Phase = "human_active"
)

type Status struct {
	Phase  Phase     `json:"phase"`
	Reason string    `json:"reason,omitempty"`
	Since  time.Time `json:"since"`
}

type State struct {
	mu     sync.RWMutex
	status Status
	now    func() time.Time
}

func New() *State {
	now := time.Now
	return &State{status: Status{Phase: AutomationActive, Since: now().UTC()}, now: now}
}

func (s *State) Request(reason string) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Phase != HumanActive {
		s.status = Status{Phase: HumanActive, Reason: reason, Since: s.now().UTC()}
	}
	return s.status
}

func (s *State) Resume() (Status, error) {
	return s.ClaimAutomation(), nil
}

// ClaimAutomation hands the browser back to the agent. Human control is a
// cooperative handoff marker rather than a persistent lock: a new MCP browser
// call is itself an explicit signal that the user wants the agent to continue.
func (s *State) ClaimAutomation() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Phase == HumanActive {
		s.status = Status{Phase: AutomationActive, Since: s.now().UTC()}
	}
	return s.status
}

func (s *State) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *State) RequireAutomation() error {
	s.ClaimAutomation()
	return nil
}
