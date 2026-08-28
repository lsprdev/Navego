package loginapproval

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lsprdev/Navego/internal/browser"
	"github.com/lsprdev/Navego/internal/credentials"
)

type Approval struct {
	ID        string                   `json:"approval_id"`
	Account   credentials.Descriptor   `json:"account"`
	Target    browser.SavedLoginTarget `json:"target"`
	ExpiresAt time.Time                `json:"expires_at"`
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

func (s *Store) Prepare(account credentials.Descriptor, target browser.SavedLoginTarget) (Approval, error) {
	if strings.TrimSpace(account.ID) == "" || account.Origin != target.Origin {
		return Approval{}, errors.New("saved login does not match the prepared browser origin")
	}
	id, err := randomID()
	if err != nil {
		return Approval{}, fmt.Errorf("create saved-login approval ID: %w", err)
	}
	prepared := Approval{
		ID:        id,
		Account:   account,
		Target:    target,
		ExpiresAt: s.now().UTC().Add(s.ttl),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	s.pending[id] = prepared
	return prepared, nil
}

func (s *Store) Take(id string) (Approval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	prepared, ok := s.pending[strings.TrimSpace(id)]
	if !ok {
		return Approval{}, errors.New("saved-login approval is unknown, expired, or already used")
	}
	delete(s.pending, prepared.ID)
	return prepared, nil
}

func (s *Store) Cancel(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	_, ok := s.pending[id]
	delete(s.pending, id)
	return ok
}

func (s *Store) cleanupLocked() {
	now := s.now().UTC()
	for id, prepared := range s.pending {
		if !prepared.ExpiresAt.After(now) {
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
