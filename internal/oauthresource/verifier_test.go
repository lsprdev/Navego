package oauthresource

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

func TestNewDiscoversProviderAndRequiresPKCES256(t *testing.T) {
	for _, tc := range []struct {
		name          string
		methods       []string
		wantErrorText string
	}{
		{name: "supported", methods: []string{"S256"}},
		{name: "missing", methods: []string{"plain"}, wantErrorText: "S256"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const issuer = "https://tenant.example.com"
			metadata, err := json.Marshal(map[string]any{
				"issuer":                                issuer,
				"authorization_endpoint":                issuer + "/authorize",
				"token_endpoint":                        issuer + "/oauth/token",
				"jwks_uri":                              issuer + "/.well-known/jwks.json",
				"response_types_supported":              []string{"code"},
				"subject_types_supported":               []string{"public"},
				"id_token_signing_alg_values_supported": []string{"RS256"},
				"code_challenge_methods_supported":      tc.methods,
				"client_id_metadata_document_supported": true,
			})
			if err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != issuer+"/.well-known/openid-configuration" {
					t.Fatalf("unexpected discovery request: %s", request.URL)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(string(metadata))),
					Request:    request,
				}, nil
			})}

			_, discovery, err := New(t.Context(), Config{
				Issuer:          issuer,
				Audience:        "https://mcp.browser.lspr.dev/mcp",
				AllowedSubjects: []string{"auth0|owner"},
			}, client)
			if tc.wantErrorText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrorText) {
					t.Fatalf("error = %v, want text %q", err, tc.wantErrorText)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !discovery.ClientIDMetadataDocumentSupported {
				t.Fatal("CIMD discovery capability was not preserved")
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestVerifierValidatesIdentityAudienceExpiryAndScopes(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const (
		issuer   = "https://tenant.example.com/"
		audience = "https://mcp.browser.lspr.dev/mcp"
		subject  = "auth0|owner"
	)
	jwtVerifier := oidc.NewVerifier(issuer, &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&privateKey.PublicKey}}, &oidc.Config{
		ClientID:             audience,
		SupportedSigningAlgs: []string{oidc.RS256},
	})
	verifier := &Verifier{
		issuer:          issuer,
		audience:        audience,
		allowedSubjects: map[string]struct{}{subject: {}},
		jwt:             jwtVerifier,
	}

	raw := signedToken(t, privateKey, map[string]any{
		"iss":         issuer,
		"aud":         audience,
		"sub":         subject,
		"iat":         time.Now().Add(-time.Minute).Unix(),
		"exp":         time.Now().Add(time.Hour).Unix(),
		"scope":       "browser:read browser:capture",
		"permissions": []string{"browser:interact", "browser:read"},
		"scp":         []string{"browser:takeover"},
	})
	info, err := verifier.Verify(context.Background(), raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.UserID != subject || info.Expiration.IsZero() {
		t.Fatalf("unexpected token info: %+v", info)
	}
	wantScopes := []string{"browser:capture", "browser:interact", "browser:read", "browser:takeover"}
	if len(info.Scopes) != len(wantScopes) {
		t.Fatalf("scopes = %v, want %v", info.Scopes, wantScopes)
	}
	for index := range wantScopes {
		if info.Scopes[index] != wantScopes[index] {
			t.Fatalf("scopes = %v, want %v", info.Scopes, wantScopes)
		}
	}
}

func TestVerifierRejectsUnallowedSubject(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const issuer = "https://tenant.example.com/"
	const audience = "https://mcp.browser.lspr.dev/mcp"
	verifier := &Verifier{
		issuer:          issuer,
		audience:        audience,
		allowedSubjects: map[string]struct{}{"auth0|owner": {}},
		jwt: oidc.NewVerifier(issuer, &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&privateKey.PublicKey}}, &oidc.Config{
			ClientID:             audience,
			SupportedSigningAlgs: []string{oidc.RS256},
		}),
	}
	raw := signedToken(t, privateKey, map[string]any{
		"iss": issuer,
		"aud": audience,
		"sub": "auth0|someone-else",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	_, err = verifier.Verify(context.Background(), raw, nil)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("error = %v, want ErrInvalidToken", err)
	}
}

func TestVerifierRejectsInvalidAudienceAndExpiredToken(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const issuer = "https://tenant.example.com/"
	const audience = "https://mcp.browser.lspr.dev/mcp"
	verifier := &Verifier{
		issuer:          issuer,
		audience:        audience,
		allowedSubjects: map[string]struct{}{"auth0|owner": {}},
		jwt: oidc.NewVerifier(issuer, &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&privateKey.PublicKey}}, &oidc.Config{
			ClientID:             audience,
			SupportedSigningAlgs: []string{oidc.RS256},
		}),
	}
	for _, tc := range []struct {
		name     string
		audience string
		expiry   time.Time
	}{
		{name: "wrong audience", audience: "https://other.example/mcp", expiry: time.Now().Add(time.Hour)},
		{name: "expired", audience: audience, expiry: time.Now().Add(-time.Hour)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := signedToken(t, privateKey, map[string]any{
				"iss": issuer,
				"aud": tc.audience,
				"sub": "auth0|owner",
				"exp": tc.expiry.Unix(),
			})
			_, err := verifier.Verify(context.Background(), raw, nil)
			if !errors.Is(err, auth.ErrInvalidToken) {
				t.Fatalf("error = %v, want ErrInvalidToken", err)
			}
		})
	}
}

func signedToken(t *testing.T, privateKey *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: privateKey}, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
