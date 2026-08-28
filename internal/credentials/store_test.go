package credentials

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMatchesExactOriginAndReadsSecrets(t *testing.T) {
	directory := t.TempDir()
	usernamePath := filepath.Join(directory, "username")
	passwordPath := filepath.Join(directory, "password")
	manifestPath := filepath.Join(directory, "logins.json")
	if err := os.WriteFile(usernamePath, []byte("owner@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passwordPath, []byte("correct horse battery staple\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":1,"logins":[{"id":"college","label":"College","origin":"https://portal.example.com/","username_file":` + quote(usernamePath) + `,"password_file":` + quote(passwordPath) + `}]}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := Load(manifestPath, directory)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := store.MatchURL("https://portal.example.com/login?next=%2Fgrades")
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.ID != "college" || descriptor.Origin != "https://portal.example.com" {
		t.Fatalf("descriptor = %+v", descriptor)
	}
	secret, err := store.ReadSecret(descriptor.ID, "https://portal.example.com/login")
	if err != nil {
		t.Fatal(err)
	}
	defer secret.Clear()
	if string(secret.Username) != "owner@example.com" || string(secret.Password) != "correct horse battery staple" {
		t.Fatal("secret contents were not read exactly")
	}
	if _, err := store.ReadSecret(descriptor.ID, "https://evil.example/login"); err == nil {
		t.Fatal("credential was allowed on a different origin")
	}
}

func TestLoadRejectsDuplicateOriginsAndEscapingPaths(t *testing.T) {
	directory := t.TempDir()
	tests := []string{
		`{"version":1,"logins":[{"id":"one","label":"One","origin":"https://example.com","username_file":"/tmp/outside","password_file":"/tmp/outside"}]}`,
		`{"version":1,"logins":[{"id":"one","label":"One","origin":"https://example.com","username_file":` + quote(filepath.Join(directory, "u")) + `,"password_file":` + quote(filepath.Join(directory, "p")) + `},{"id":"two","label":"Two","origin":"https://example.com/","username_file":` + quote(filepath.Join(directory, "u2")) + `,"password_file":` + quote(filepath.Join(directory, "p2")) + `}]}`,
	}
	for _, manifest := range tests {
		path := filepath.Join(directory, "manifest.json")
		if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path, directory); err == nil {
			t.Fatalf("manifest should fail: %s", manifest)
		}
	}
}

func TestOriginNormalizationIsStrict(t *testing.T) {
	origin, err := CanonicalOrigin("https://EXAMPLE.com:443/")
	if err != nil || origin != "https://example.com" {
		t.Fatalf("origin = %q, err = %v", origin, err)
	}
	for _, raw := range []string{"http://example.com", "https://example.com/login", "https://user@example.com", "https://example.com?x=1"} {
		if _, err := CanonicalOrigin(raw); err == nil {
			t.Fatalf("origin %q should be rejected", raw)
		}
	}
}

func TestSecretFilesCannotEscapeThroughSymlink(t *testing.T) {
	directory := t.TempDir()
	outside := t.TempDir()
	outsideUsername := filepath.Join(outside, "username")
	if err := os.WriteFile(outsideUsername, []byte("should-not-be-read"), 0o600); err != nil {
		t.Fatal(err)
	}
	usernamePath := filepath.Join(directory, "username")
	if err := os.Symlink(outsideUsername, usernamePath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	passwordPath := filepath.Join(directory, "password")
	if err := os.WriteFile(passwordPath, []byte("password"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(directory, "logins.json")
	manifest := `{"version":1,"logins":[{"id":"test","label":"Test","origin":"https://example.com","username_file":` + quote(usernamePath) + `,"password_file":` + quote(passwordPath) + `}]}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Load(manifestPath, directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadSecret("test", "https://example.com/login"); err == nil {
		t.Fatal("symlink escaping the secrets directory should be rejected")
	}
}

func quote(value string) string {
	return `"` + strings.ReplaceAll(value, `\`, `\\`) + `"`
}
