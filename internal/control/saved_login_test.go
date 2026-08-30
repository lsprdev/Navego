package control

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/lsprdev/Navego/internal/browser"
	"github.com/lsprdev/Navego/internal/httpserver"
	"github.com/lsprdev/Navego/pb_migrations"
	"github.com/pocketbase/pocketbase/core"
)

type savedLoginRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn savedLoginRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestExecuteSavedLoginBrokersOwnerCredentialWithoutLeakingIt(t *testing.T) {
	vaultKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	app := New(Config{DataDir: t.TempDir(), VaultKey: vaultKey})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.Set("name", "Student")
	user.Set("email", "student@example.com")
	user.Set("password", "student-password-123")
	user.Set("passwordConfirm", "student-password-123")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	browsers, err := app.FindCollectionByNameOrId(pb_migrations.BrowsersCollection)
	if err != nil {
		t.Fatal(err)
	}
	browserRecord := core.NewRecord(browsers)
	browserRecord.Set("owner", user.Id)
	browserRecord.Set("name", "Main")
	browserRecord.Set("state", "running")
	browserRecord.Set("worker_endpoint", "http://navego-browser-test123:8001")
	if err := app.Save(browserRecord); err != nil {
		t.Fatal(err)
	}
	user.Set("default_browser", browserRecord.Id)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	vault, err := newCredentialVault(vaultKey)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := vault.encrypt(user.Id, "https://sig.ifc.edu.br", credentialSecret{
		Username: "student-user", Password: "vault-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	credentialCollection, err := app.FindCollectionByNameOrId(pb_migrations.CredentialsCollection)
	if err != nil {
		t.Fatal(err)
	}
	credentialRecord := core.NewRecord(credentialCollection)
	credentialRecord.Set("owner", user.Id)
	credentialRecord.Set("label", "Site da Faculdade")
	credentialRecord.Set("origin", "https://sig.ifc.edu.br")
	credentialRecord.Set("encrypted_payload", payload)
	credentialRecord.Set("key_version", credentialKeyVersion)
	if err := app.Save(credentialRecord); err != nil {
		t.Fatal(err)
	}

	var committedUsername, committedPassword []byte
	workerClient := &http.Client{Transport: savedLoginRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer worker-secret" {
			t.Fatalf("worker authorization = %q", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/internal/saved-login/describe":
			var input httpserver.InternalSavedLoginDescribeRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input.UsernameRef != "g1e1" || input.PasswordRef != "g1e2" || input.SubmitRef != "g1e3" {
				t.Fatalf("unexpected describe refs: %#v", input)
			}
			return savedLoginHTTPResponse(http.StatusOK, httpserver.InternalSavedLoginDescribeResponse{
				Target: httpserver.InternalSavedLoginTarget{
					URL: "https://sig.ifc.edu.br/sigaa/verTelaLogin.do", RawURL: "https://sig.ifc.edu.br/sigaa/verTelaLogin.do",
					Origin: "https://sig.ifc.edu.br", Generation: 4,
					UsernameRef: "g1e1", UsernameName: "Usuário",
					PasswordRef: "g1e2", PasswordName: "Senha",
					SubmitRef: "g1e3", SubmitName: "Entrar",
				},
			})
		case "/internal/saved-login/commit":
			var input httpserver.InternalSavedLoginCommitRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			committedUsername = append([]byte(nil), input.Username...)
			committedPassword = append([]byte(nil), input.Password...)
			input.Clear()
			return savedLoginHTTPResponse(http.StatusOK, httpserver.InternalSavedLoginCommitResponse{
				Snapshot: browser.Snapshot{URL: "https://sig.ifc.edu.br/sigaa/portais/discente/discente.jsf", Title: "Portal do Discente"},
			})
		default:
			t.Fatalf("unexpected worker path %q", request.URL.Path)
			return nil, nil
		}
	})}

	service := &multiBrowserMCP{
		app:   app,
		cfg:   Config{WorkerAPIKey: "worker-secret", InternalHTTP: workerClient},
		vault: vault,
	}
	result, usedBrowserID := service.executeSavedLogin(t.Context(), user.Id, savedLoginToolInput{
		UsernameRef: "g1e1", PasswordRef: "g1e2", SubmitRef: "g1e3",
	})
	if result.IsError || usedBrowserID != browserRecord.Id {
		t.Fatalf("unexpected result: browser=%q result=%#v", usedBrowserID, result)
	}
	if string(committedUsername) != "student-user" || string(committedPassword) != "vault-password" {
		t.Fatal("control plane did not broker the saved credential")
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"student-user", "vault-password", payload} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("public MCP result leaked secret material %q", secret)
		}
	}
	if !bytes.Contains(encoded, []byte("Site da Faculdade")) || !bytes.Contains(encoded, []byte("https://sig.ifc.edu.br")) {
		t.Fatalf("result is missing safe account metadata: %s", encoded)
	}
	foreignMaterial, err := service.savedCredential("different-owner", "https://sig.ifc.edu.br")
	foreignMaterial.Clear()
	if !errors.Is(err, errSavedCredentialNotFound) {
		t.Fatalf("credential lookup crossed the OAuth owner boundary: %v", err)
	}
}

func TestExecuteSavedLoginDoesNotSendCredentialAcrossOrigins(t *testing.T) {
	vaultKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))
	app := New(Config{DataDir: t.TempDir(), VaultKey: vaultKey})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })

	user, browserRecord, vault := savedLoginFixture(t, app, vaultKey)
	commitCalled := false
	client := &http.Client{Transport: savedLoginRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/internal/saved-login/commit" {
			commitCalled = true
			t.Fatal("credential must not be sent to a different origin")
		}
		return savedLoginHTTPResponse(http.StatusOK, httpserver.InternalSavedLoginDescribeResponse{
			Target: httpserver.InternalSavedLoginTarget{
				URL: "https://evil.example/login", RawURL: "https://evil.example/login", Origin: "https://evil.example",
				Generation: 1, UsernameRef: "g1e1", PasswordRef: "g1e2", SubmitRef: "g1e3", SubmitName: "Sign in",
			},
		})
	})}
	service := &multiBrowserMCP{app: app, cfg: Config{WorkerAPIKey: "worker-secret", InternalHTTP: client}, vault: vault}
	result, usedBrowserID := service.executeSavedLogin(t.Context(), user.Id, savedLoginToolInput{
		UsernameRef: "g1e1", PasswordRef: "g1e2", SubmitRef: "g1e3",
	})
	if !result.IsError || usedBrowserID != browserRecord.Id || commitCalled {
		t.Fatalf("cross-origin login was not blocked: browser=%q result=%#v", usedBrowserID, result)
	}
}

func savedLoginFixture(t *testing.T, app core.App, vaultKey string) (*core.Record, *core.Record, *credentialVault) {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.Set("name", "Owner")
	user.Set("email", "owner@example.com")
	user.Set("password", "owner-password-123")
	user.Set("passwordConfirm", "owner-password-123")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	browsers, _ := app.FindCollectionByNameOrId(pb_migrations.BrowsersCollection)
	browserRecord := core.NewRecord(browsers)
	browserRecord.Set("owner", user.Id)
	browserRecord.Set("name", "Main")
	browserRecord.Set("state", "running")
	browserRecord.Set("worker_endpoint", "http://navego-browser-test456:8001")
	if err := app.Save(browserRecord); err != nil {
		t.Fatal(err)
	}
	user.Set("default_browser", browserRecord.Id)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	vault, err := newCredentialVault(vaultKey)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := vault.encrypt(user.Id, "https://sig.ifc.edu.br", credentialSecret{Username: "owner", Password: "secret"})
	credentialsCollection, _ := app.FindCollectionByNameOrId(pb_migrations.CredentialsCollection)
	record := core.NewRecord(credentialsCollection)
	record.Set("owner", user.Id)
	record.Set("label", "SIGAA")
	record.Set("origin", "https://sig.ifc.edu.br")
	record.Set("encrypted_payload", payload)
	record.Set("key_version", credentialKeyVersion)
	if err := app.Save(record); err != nil {
		t.Fatal(err)
	}
	return user, browserRecord, vault
}

func savedLoginHTTPResponse(status int, value any) (*http.Response, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}, nil
}
