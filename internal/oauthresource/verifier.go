package oauthresource

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

// Config describes the authorization server and the single-user boundary that
// every MCP access token must satisfy.
type Config struct {
	Issuer          string
	Audience        string
	AllowedSubjects []string
}

// Discovery summarizes the MCP-relevant capabilities advertised by the
// authorization server during startup discovery.
type Discovery struct {
	ClientIDMetadataDocumentSupported bool
	RegistrationEndpoint              string
}

// Verifier validates signed JWT access tokens using the authorization server's
// discovered JWKS and converts their claims into the MCP SDK's TokenInfo.
type Verifier struct {
	issuer          string
	audience        string
	allowedSubjects map[string]struct{}
	jwt             *oidc.IDTokenVerifier
}

// New discovers an OpenID Connect provider, verifies that it advertises PKCE
// S256, and constructs a JWT/JWKS verifier. The supplied HTTP client is reused
// for discovery and JWKS rotation.
func New(ctx context.Context, cfg Config, client *http.Client) (*Verifier, Discovery, error) {
	if client == nil {
		client = http.DefaultClient
	}
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, client), cfg.Issuer)
	if err != nil {
		return nil, Discovery{}, fmt.Errorf("discover OAuth issuer: %w", err)
	}

	var metadata struct {
		CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
		ClientIDMetadataDocumentSupported bool     `json:"client_id_metadata_document_supported"`
		RegistrationEndpoint              string   `json:"registration_endpoint"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return nil, Discovery{}, fmt.Errorf("decode OAuth discovery metadata: %w", err)
	}
	if !slices.Contains(metadata.CodeChallengeMethodsSupported, "S256") {
		return nil, Discovery{}, fmt.Errorf("OAuth issuer does not advertise required PKCE method S256")
	}

	allowed := make(map[string]struct{}, len(cfg.AllowedSubjects))
	for _, subject := range cfg.AllowedSubjects {
		if subject = strings.TrimSpace(subject); subject != "" {
			allowed[subject] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return nil, Discovery{}, fmt.Errorf("at least one OAuth subject must be allowed")
	}

	verifierContext := oidc.ClientContext(context.Background(), client)
	verifier := &Verifier{
		issuer:          cfg.Issuer,
		audience:        cfg.Audience,
		allowedSubjects: allowed,
		jwt: provider.VerifierContext(verifierContext, &oidc.Config{
			ClientID: cfg.Audience,
		}),
	}
	return verifier, Discovery{
		ClientIDMetadataDocumentSupported: metadata.ClientIDMetadataDocumentSupported,
		RegistrationEndpoint:              metadata.RegistrationEndpoint,
	}, nil
}

// Verify implements auth.TokenVerifier.
func (v *Verifier) Verify(ctx context.Context, rawToken string, _ *http.Request) (*auth.TokenInfo, error) {
	token, err := v.jwt.Verify(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("%w: JWT verification failed", auth.ErrInvalidToken)
	}
	if token.Subject == "" {
		return nil, fmt.Errorf("%w: token subject is missing", auth.ErrInvalidToken)
	}
	if _, ok := v.allowedSubjects[token.Subject]; !ok {
		return nil, fmt.Errorf("%w: token subject is not allowed", auth.ErrInvalidToken)
	}

	var claims struct {
		Scope       string   `json:"scope"`
		SCP         any      `json:"scp"`
		Permissions []string `json:"permissions"`
	}
	if err := token.Claims(&claims); err != nil {
		return nil, fmt.Errorf("%w: access token claims are invalid", auth.ErrInvalidToken)
	}

	scopes := strings.Fields(claims.Scope)
	scopes = append(scopes, scopeValues(claims.SCP)...)
	scopes = append(scopes, claims.Permissions...)
	scopes = uniqueStrings(scopes)

	return &auth.TokenInfo{
		Scopes:     scopes,
		Expiration: token.Expiry,
		UserID:     token.Subject,
		Extra: map[string]any{
			"issuer":   v.issuer,
			"audience": v.audience,
		},
	}, nil
}

func scopeValues(value any) []string {
	switch value := value.(type) {
	case string:
		return strings.Fields(value)
	case []any:
		values := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
		return values
	default:
		return nil
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
