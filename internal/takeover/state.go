package takeover

import (
	"errors"
	"sync"
	"time"
)

var ErrHumanControlActive = errors.New("human control is active; wait for the user to finish and call browser_resume_after_human")

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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Phase != HumanActive {
		return s.status, errors.New("no human takeover is active")
	}
	s.status = Status{Phase: AutomationActive, Since: s.now().UTC()}
	return s.status, nil
}

func (s *State) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *State) RequireAutomation() error {
	if s.Status().Phase == HumanActive {
		return ErrHumanControlActive
	}
	return nil
}
