package control

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/lsprdev/Navego/pb_migrations"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

func securityApp(t *testing.T, cfg Config) (core.App, string) {
	t.Helper()
	cfg.DataDir = t.TempDir()
	app := New(cfg)
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.OnServe().Trigger(&core.ServeEvent{App: app, Router: router}); err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return app, server.URL
}

func securityUser(t *testing.T, app core.App, email string, verified bool) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	u := core.NewRecord(collection)
	u.SetEmail(email)
	u.SetPassword("test-password-12345")
	u.SetVerified(verified)
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	return u
}

func securityRequest(t *testing.T, method, url, token string, body any) int {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestEmailPolicyIsExactAndFailsClosed(t *testing.T) {
	p, err := parseAccessPolicy(" Alice@Example.com , bob@example.com\n")
	if err != nil {
		t.Fatal(err)
	}
	if !p.allowsEmail("ALICE@example.com") {
		t.Fatal("email normalization failed")
	}
	for _, email := range []string{"mallory@example.com", "alice+other@example.com", "alice@example.com.attacker.test", ""} {
		if p.allowsEmail(email) {
			t.Fatalf("accepted unlisted email %q", email)
		}
	}
	empty, _ := parseAccessPolicy("")
	if empty.allowsEmail("alice@example.com") {
		t.Fatal("empty whitelist allowed access")
	}
	for _, raw := range []string{"*", "*@example.com", "Alice <alice@example.com>", "invalid"} {
		if _, err := parseAccessPolicy(raw); err == nil {
			t.Fatalf("accepted invalid config %q", raw)
		}
	}
}

func TestAccessPolicyCoversSignupLoginRefreshAndExistingTokens(t *testing.T) {
	app, base := securityApp(t, Config{AllowedEmails: "alice@example.com,bob@example.com,charlie@example.com"})
	signup := func(email string) int {
		return securityRequest(t, "POST", base+"/api/collections/users/records", "", map[string]any{"email": email, "password": "test-password-12345", "passwordConfirm": "test-password-12345"})
	}
	if got := signup("outsider@example.com"); got != 403 {
		t.Fatalf("unlisted signup: %d", got)
	}
	if got := signup("alice@example.com"); got != 200 {
		t.Fatalf("listed signup: %d", got)
	}
	alice, err := app.FindAuthRecordByEmail("users", "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	login := func(email string) int {
		return securityRequest(t, "POST", base+"/api/collections/users/auth-with-password", "", map[string]string{"identity": email, "password": "test-password-12345"})
	}
	if got := login(alice.Email()); got != 403 {
		t.Fatalf("unverified login: %d", got)
	}
	alice.SetVerified(true)
	if err := app.Save(alice); err != nil {
		t.Fatal(err)
	}
	if got := login(alice.Email()); got != 200 {
		t.Fatalf("verified login: %d", got)
	}
	for _, u := range []*core.Record{alice, securityUser(t, app, "outsider@example.com", true), securityUser(t, app, "bob@example.com", false)} {
		token, err := u.NewAuthToken()
		if err != nil {
			t.Fatal(err)
		}
		want := 403
		if u.Id == alice.Id {
			want = 200
		}
		for _, path := range []string{"/api/navego/browsers", "/api/collections/browsers/records", "/api/collections/users/auth-refresh"} {
			method := "GET"
			if path == "/api/collections/users/auth-refresh" {
				method = "POST"
			}
			if got := securityRequest(t, method, base+path, token, nil); got != want {
				t.Fatalf("%s %s: got %d want %d", u.Email(), path, got, want)
			}
		}
	}
	token, _ := alice.NewAuthToken()
	if got := securityRequest(t, "POST", base+"/api/navego/browsers", token, map[string]string{"name": "Primeiro"}); got != 202 {
		t.Fatalf("create through quota API: %d", got)
	}
	if got := securityRequest(t, "DELETE", base+"/api/collections/users/records/"+alice.Id, token, nil); got < 400 {
		t.Fatal("account deletion orphaned containers and bypassed capacity tracking")
	}
	if got := securityRequest(t, "POST", base+"/api/navego/browsers", token, map[string]string{"name": "Segundo"}); got != 202 {
		t.Fatalf("second browser: %d", got)
	}
	if got := securityRequest(t, "POST", base+"/api/navego/browsers", token, map[string]string{"name": "Terceiro"}); got != 409 {
		t.Fatalf("quota HTTP status: %d", got)
	}
	if got := securityRequest(t, "POST", base+"/api/collections/browsers/records", token, map[string]string{"owner": alice.Id, "name": "Bypass", "state": "queued"}); got < 400 {
		t.Fatal("native records API bypassed quota route")
	}
	if got := securityRequest(t, "POST", base+"/api/collections/users/records", "", map[string]any{"email": "charlie@example.com", "password": "test-password-12345", "passwordConfirm": "test-password-12345", "verified": true}); got < 400 {
		t.Fatal("signup accepted self-verification")
	}
}

func TestConcurrentBrowserQuotaIncludesPendingDeletion(t *testing.T) {
	app, _ := securityApp(t, Config{})
	a := securityUser(t, app, "a@example.com", true)
	b := securityUser(t, app, "b@example.com", true)
	cfg := Config{MaxBrowsersPerUser: 2, MaxBrowsersTotal: 3}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			owner := a.Id
			if i%2 == 0 {
				owner = b.Id
			}
			_, _, _ = reserveBrowser(app, owner, fmt.Sprintf("Browser %d", i), cfg)
		}(i)
	}
	wg.Wait()
	records, err := app.FindAllRecords(pb_migrations.BrowsersCollection)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("global quota: got %d records", len(records))
	}
	counts := map[string]int{}
	for _, record := range records {
		counts[record.GetString("owner")]++
	}
	if counts[a.Id] > 2 || counts[b.Id] > 2 {
		t.Fatalf("per-user quota bypassed: %v", counts)
	}
	for _, u := range []*core.Record{a, b} {
		fresh, err := app.FindRecordById("users", u.Id)
		if err != nil || fresh.GetString("default_browser") == "" {
			t.Fatal("atomic default selection failed")
		}
	}
	records[0].Set("state", "deleting")
	if err := app.Save(records[0]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reserveBrowser(app, a.Id, "Must not fit", cfg); err == nil {
		t.Fatal("pending deletion freed capacity prematurely")
	}
	owner := records[0].GetString("owner")
	if err := app.Delete(records[0]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reserveBrowser(app, owner, "Replacement", cfg); err != nil {
		t.Fatalf("confirmed deletion did not free capacity: %v", err)
	}
}
