package control

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCredentialVaultRoundTripAndOwnership(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	vault, err := newCredentialVault(key)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := vault.encrypt("owner-one", "https://example.com", credentialSecret{
		Username: "person@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload, "person@example.com") || strings.Contains(payload, "correct horse") {
		t.Fatal("plaintext leaked into encrypted payload")
	}
	secret, err := vault.decrypt("owner-one", "https://example.com", payload)
	if err != nil {
		t.Fatal(err)
	}
	if secret.Username != "person@example.com" || secret.Password != "correct horse battery staple" {
		t.Fatalf("unexpected decrypted secret: %#v", secret)
	}
	if _, err := vault.decrypt("owner-two", "https://example.com", payload); err == nil {
		t.Fatal("a different owner must not decrypt the payload")
	}
	if _, err := vault.decrypt("owner-one", "https://other.example", payload); err == nil {
		t.Fatal("a different origin must not decrypt the payload")
	}
}

func TestCredentialVaultRejectsInvalidKeysAndTampering(t *testing.T) {
	if _, err := newCredentialVault(""); err == nil {
		t.Fatal("missing key must fail")
	}
	if _, err := newCredentialVault(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("short key must fail")
	}

	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	vault, err := newCredentialVault(key)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := vault.encrypt("owner", "https://example.com", credentialSecret{Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	last := payload[len(payload)-1]
	replacement := byte('A')
	if last == replacement {
		replacement = 'B'
	}
	tampered := payload[:len(payload)-1] + string(replacement)
	if _, err := vault.decrypt("owner", "https://example.com", tampered); err == nil {
		t.Fatal("tampered payload must fail authentication")
	}
}
