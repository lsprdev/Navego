package approval

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lsprdev/Navego/internal/browser"
)

type Approval struct {
	ID        string               `json:"approval_id"`
	Summary   string               `json:"summary"`
	Target    browser.ActionTarget `json:"target"`
	ExpiresAt time.Time            `json:"expires_at"`
}

type Store struct {
	mu      sync.Mutex
	ttl     time.Duration
	now     func() time.Time
	pending map[string]Approval
}

func NewStore(ttl time.Duration) *Store {
	return &Store{ttl: ttl, now: time.Now, pending: make(map[string]Approval)}
}

func (s *Store) Prepare(summary string, target browser.ActionTarget) (Approval, error) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return Approval{}, errors.New("action summary is required")
	}
	if len(summary) > 500 {
		return Approval{}, errors.New("action summary is too long")
	}
	id, err := randomID()
	if err != nil {
		return Approval{}, fmt.Errorf("create approval ID: %w", err)
	}
	approval := Approval{
		ID:        id,
		Summary:   summary,
		Target:    target,
		ExpiresAt: s.now().UTC().Add(s.ttl),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	s.pending[id] = approval
	return approval, nil
}

func (s *Store) Take(id string) (Approval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	approval, ok := s.pending[id]
	if !ok {
		return Approval{}, errors.New("approval is unknown, expired, or already used")
	}
	delete(s.pending, id)
	return approval, nil
}

func (s *Store) Cancel(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.pending[id]
	delete(s.pending, id)
	return ok
}

func (s *Store) cleanupLocked() {
	now := s.now().UTC()
	for id, approval := range s.pending {
		if !approval.ExpiresAt.After(now) {
			delete(s.pending, id)
		}
	}
}

func randomID() (string, error) {
	var raw [18]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
