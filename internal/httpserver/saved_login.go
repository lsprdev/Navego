package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/lsprdev/Navego/internal/browser"
)

const maxInternalSavedLoginRequestBytes = 64 << 10

// InternalSavedLoginDescribeRequest contains only model-visible element refs.
// The worker resolves them against its current page before the control plane
// opens the user's encrypted credential vault.
type InternalSavedLoginDescribeRequest struct {
	UsernameRef string `json:"username_ref"`
	PasswordRef string `json:"password_ref"`
	SubmitRef   string `json:"submit_ref"`
}

// InternalSavedLoginTarget preserves the exact, unredacted page URL only on
// Navego's authenticated internal hop. It is never returned by the public MCP.
type InternalSavedLoginTarget struct {
	URL          string `json:"url"`
	RawURL       string `json:"raw_url"`
	Origin       string `json:"origin"`
	Generation   uint64 `json:"generation"`
	UsernameRef  string `json:"username_ref"`
	UsernameName string `json:"username_name,omitempty"`
	PasswordRef  string `json:"password_ref"`
	PasswordName string `json:"password_name,omitempty"`
	SubmitRef    string `json:"submit_ref"`
	SubmitName   string `json:"submit_name"`
}

type InternalSavedLoginDescribeResponse struct {
	Target InternalSavedLoginTarget `json:"target"`
}

// []byte values are encoded as base64 by encoding/json. They exist only for
// the duration of the authenticated control-plane-to-worker request and are
// cleared immediately after use.
type InternalSavedLoginCommitRequest struct {
	Target   InternalSavedLoginTarget `json:"target"`
	Username []byte                   `json:"username"`
	Password []byte                   `json:"password"`
}

func (r *InternalSavedLoginCommitRequest) Clear() {
	clear(r.Username)
	clear(r.Password)
	r.Username = nil
	r.Password = nil
}

type InternalSavedLoginCommitResponse struct {
	Snapshot browser.Snapshot `json:"snapshot"`
}

func internalSavedLoginDescribeHandler(controller browser.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		loginController, ok := controller.(browser.SavedLoginController)
		if !ok {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "saved login is not supported"})
			return
		}
		var input InternalSavedLoginDescribeRequest
		if err := decodeInternalSavedLoginJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid saved-login request"})
			return
		}
		target, err := loginController.DescribeSavedLogin(
			r.Context(),
			strings.TrimSpace(input.UsernameRef),
			strings.TrimSpace(input.PasswordRef),
			strings.TrimSpace(input.SubmitRef),
		)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, InternalSavedLoginDescribeResponse{Target: internalSavedLoginTarget(target)})
	}
}

func internalSavedLoginCommitHandler(controller browser.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		loginController, ok := controller.(browser.SavedLoginController)
		if !ok {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "saved login is not supported"})
			return
		}
		var input InternalSavedLoginCommitRequest
		if err := decodeInternalSavedLoginJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid saved-login request"})
			return
		}
		defer input.Clear()
		if len(input.Username) == 0 || len(input.Username) > 16<<10 || len(input.Password) == 0 || len(input.Password) > 16<<10 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid saved-login secret size"})
			return
		}
		snapshot, err := loginController.CommitSavedLogin(
			r.Context(),
			input.Target.browserTarget(),
			input.Username,
			input.Password,
		)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, InternalSavedLoginCommitResponse{Snapshot: snapshot})
	}
}

func decodeInternalSavedLoginJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxInternalSavedLoginRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON document")
	}
	return nil
}

func internalSavedLoginTarget(target browser.SavedLoginTarget) InternalSavedLoginTarget {
	return InternalSavedLoginTarget{
		URL: target.URL, RawURL: target.RawURL, Origin: target.Origin, Generation: target.Generation,
		UsernameRef: target.UsernameRef, UsernameName: target.UsernameName,
		PasswordRef: target.PasswordRef, PasswordName: target.PasswordName,
		SubmitRef: target.SubmitRef, SubmitName: target.SubmitName,
	}
}

func (target InternalSavedLoginTarget) browserTarget() browser.SavedLoginTarget {
	return browser.SavedLoginTarget{
		URL: target.URL, RawURL: target.RawURL, Origin: target.Origin, Generation: target.Generation,
		UsernameRef: target.UsernameRef, UsernameName: target.UsernameName,
		PasswordRef: target.PasswordRef, PasswordName: target.PasswordName,
		SubmitRef: target.SubmitRef, SubmitName: target.SubmitName,
	}
}
