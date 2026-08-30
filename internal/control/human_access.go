package control

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lsprdev/Navego/pb_migrations"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

const humanAccessTTL = 15 * time.Minute

var (
	errHumanAccessExpired  = errors.New("human access expired")
	errHumanAccessMismatch = errors.New("human access owner mismatch")
)

type humanAccessGrant struct {
	ownerID   string
	browserID string
	expiresAt time.Time
}

// humanAccessStore keeps the ChatGPT-to-dashboard handoff separate from the
// short-lived iframe tickets. Handoff links may be prefetched or revisited, so
// resolving one is intentionally non-destructive; account ownership remains
// the actual authorization boundary.
type humanAccessStore struct {
	mu      sync.Mutex
	tickets map[string]humanAccessGrant
	now     func() time.Time
}

func newHumanAccessStore() *humanAccessStore {
	return &humanAccessStore{
		tickets: make(map[string]humanAccessGrant),
		now:     time.Now,
	}
}

func (s *humanAccessStore) mint(ownerID, browserID string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	s.tickets[token] = humanAccessGrant{
		ownerID:   ownerID,
		browserID: browserID,
		expiresAt: s.now().Add(humanAccessTTL),
	}
	return token, nil
}

func (s *humanAccessStore) resolve(ticket string) (humanAccessGrant, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	grant, ok := s.tickets[strings.TrimSpace(ticket)]
	return grant, ok
}

func (s *humanAccessStore) resolveForOwner(ticket, ownerID string) (humanAccessGrant, error) {
	grant, ok := s.resolve(ticket)
	if !ok {
		return humanAccessGrant{}, errHumanAccessExpired
	}
	if grant.ownerID != ownerID {
		return humanAccessGrant{}, errHumanAccessMismatch
	}
	return grant, nil
}

func (s *humanAccessStore) cleanupLocked() {
	now := s.now()
	for token, grant := range s.tickets {
		if !grant.expiresAt.After(now) {
			delete(s.tickets, token)
		}
	}
}

func resolveHumanAccess(store *humanAccessStore) func(*core.RequestEvent) error {
	return func(event *core.RequestEvent) error {
		event.Response.Header().Set("Cache-Control", "private, no-store")
		event.Response.Header().Set("Referrer-Policy", "no-referrer")

		grant, err := store.resolveForOwner(event.Request.PathValue("ticket"), event.Auth.Id)
		if errors.Is(err, errHumanAccessExpired) {
			return apis.NewApiError(http.StatusGone, "Este link de acesso expirou. Peça um novo link no ChatGPT.", nil)
		}
		if errors.Is(err, errHumanAccessMismatch) {
			return apis.NewApiError(http.StatusForbidden, "Este link pertence a outra conta Navego.", nil)
		}

		browser, err := event.App.FindRecordById(pb_migrations.BrowsersCollection, grant.browserID)
		if err != nil || browser.GetString("owner") != event.Auth.Id || browser.GetString("state") == "deleting" {
			return event.NotFoundError("O Chromium deste acesso não existe mais.", err)
		}
		if browser.GetString("state") != "running" {
			return apis.NewApiError(http.StatusConflict, "O Chromium deste acesso não está ligado.", nil)
		}

		return event.JSON(http.StatusOK, mapBrowser(
			browser,
			browser.Id == event.Auth.GetString("default_browser"),
		))
	}
}
