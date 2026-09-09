package control

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/lsprdev/Navego/internal/browser"
	"github.com/lsprdev/Navego/internal/httpserver"
)

func TestSavedLoginDoesNotClaimAuthentication(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32))
	app := New(Config{DataDir: t.TempDir(), VaultKey: key})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	user, record, vault := savedLoginFixture(t, app, key)
	for _, test := range []struct {
		name, url, text string
		failure         bool
	}{
		{"blocked old worker", "chrome-error://chromewebdata/", "ERR_BLOCKED_BY_CLIENT", true},
		{"form still visible", "https://sig.ifc.edu.br/sigaa/verTelaLogin.do", "Usuário Senha", false},
		{"MFA", "https://sig.ifc.edu.br/mfa", "Enter code", false},
		{"portal", "https://sig.ifc.edu.br/portal", "Portal do Discente", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			commits := 0
			client := &http.Client{Transport: savedLoginRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Path == "/internal/saved-login/describe" {
					return savedLoginHTTPResponse(http.StatusOK, httpserver.InternalSavedLoginDescribeResponse{Target: httpserver.InternalSavedLoginTarget{
						RawURL: "https://sig.ifc.edu.br/login", Origin: "https://sig.ifc.edu.br",
					}})
				}
				if request.URL.Path != "/internal/saved-login/commit" {
					t.Fatalf("unexpected path: %s", request.URL.Path)
				}
				commits++
				return savedLoginHTTPResponse(http.StatusOK, httpserver.InternalSavedLoginCommitResponse{Snapshot: browser.Snapshot{URL: test.url, Text: test.text}})
			})}
			service := &multiBrowserMCP{app: app, cfg: Config{WorkerAPIKey: "worker-secret", InternalHTTP: client}, vault: vault}
			result, id := service.executeSavedLogin(t.Context(), user.Id, savedLoginToolInput{UsernameRef: "u", PasswordRef: "p", SubmitRef: "s"})
			if id != record.Id || result.IsError != test.failure || commits != 1 {
				t.Fatalf("unexpected result: %+v; commits=%d", result, commits)
			}
			encoded, _ := json.Marshal(result)
			if strings.Contains(string(encoded), `"signed_in"`) {
				t.Fatal("false authentication status")
			}
			if test.failure {
				if !strings.Contains(string(encoded), "ERR_BLOCKED_BY_CLIENT") {
					t.Fatal("missing browser failure reason")
				}
			} else if !strings.Contains(string(encoded), "submitted_unverified") {
				t.Fatal("missing unverified status")
			}
		})
	}
}
