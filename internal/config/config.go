package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host                    string
	Port                    int
	CDPEndpoint             string
	APIKey                  string
	PublicURL               string
	OAuthEnabled            bool
	OAuthIssuer             string
	OAuthAudience           string
	OAuthAllowedSubjects    []string
	OAuthDiscoveryTimeout   time.Duration
	HumanTakeoverURL        string
	ActionTimeout           time.Duration
	NavigationTimeout       time.Duration
	SessionIdleTimeout      time.Duration
	SnapshotMaxChars        int
	SnapshotMaxElements     int
	ObscuraEndpoint         string
	ObscuraMaxPayload       int
	ObscuraFailureThreshold int
	ObscuraCooldown         time.Duration
	AlwaysChromium          []string
}

func Load(lookup func(string) (string, bool)) (Config, error) {
	port, err := intValue(lookup, "MCP_PORT", 8001)
	if err != nil {
		return Config{}, err
	}
	actionTimeout, err := durationMS(lookup, "MCP_ACTION_TIMEOUT_MS", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	navigationTimeout, err := durationMS(lookup, "MCP_NAVIGATION_TIMEOUT_MS", 60*time.Second)
	if err != nil {
		return Config{}, err
	}
	sessionIdleTimeout, err := durationMS(lookup, "MCP_SESSION_IDLE_TIMEOUT_MS", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}
	maxChars, err := intValue(lookup, "MCP_SNAPSHOT_MAX_CHARS", 12_000)
	if err != nil {
		return Config{}, err
	}
	maxElements, err := intValue(lookup, "MCP_SNAPSHOT_MAX_ELEMENTS", 150)
	if err != nil {
		return Config{}, err
	}
	oauthEnabled, err := boolValue(lookup, "MCP_OAUTH_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	oauthDiscoveryTimeout, err := durationMS(lookup, "MCP_OAUTH_DISCOVERY_TIMEOUT_MS", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Host:                    stringValue(lookup, "MCP_HOST", "0.0.0.0"),
		Port:                    port,
		CDPEndpoint:             stringValue(lookup, "MCP_CDP_ENDPOINT", "http://127.0.0.1:9222"),
		APIKey:                  stringValue(lookup, "MCP_API_KEY", ""),
		PublicURL:               stringValue(lookup, "MCP_PUBLIC_URL", ""),
		OAuthEnabled:            oauthEnabled,
		OAuthIssuer:             stringValue(lookup, "MCP_OAUTH_ISSUER", ""),
		OAuthAudience:           stringValue(lookup, "MCP_OAUTH_AUDIENCE", ""),
		OAuthAllowedSubjects:    csvValue(lookup, "MCP_OAUTH_ALLOWED_SUBJECTS", ""),
		OAuthDiscoveryTimeout:   oauthDiscoveryTimeout,
		HumanTakeoverURL:        stringValue(lookup, "HUMAN_TAKEOVER_URL", "https://127.0.0.1:3001"),
		ActionTimeout:           actionTimeout,
		NavigationTimeout:       navigationTimeout,
		SessionIdleTimeout:      sessionIdleTimeout,
		SnapshotMaxChars:        maxChars,
		SnapshotMaxElements:     maxElements,
		ObscuraEndpoint:         stringValue(lookup, "OBSCURA_MCP_ENDPOINT", ""),
		ObscuraMaxPayload:       16 << 20,
		ObscuraFailureThreshold: 3,
		ObscuraCooldown:         30 * time.Second,
		AlwaysChromium:          csvValue(lookup, "MCP_ALWAYS_CHROMIUM_DOMAINS", "x.com,twitter.com,amazon.com,amazon.com.br"),
	}
	if cfg.ObscuraFailureThreshold, err = intValue(lookup, "OBSCURA_FAILURE_THRESHOLD", cfg.ObscuraFailureThreshold); err != nil {
		return Config{}, err
	}
	if cfg.ObscuraCooldown, err = durationMS(lookup, "OBSCURA_COOLDOWN_MS", cfg.ObscuraCooldown); err != nil {
		return Config{}, err
	}
	if raw, ok := lookup("OBSCURA_MAX_PAYLOAD_BYTES"); ok && strings.TrimSpace(raw) != "" {
		cfg.ObscuraMaxPayload, err = strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || cfg.ObscuraMaxPayload < 1 {
			return Config{}, fmt.Errorf("OBSCURA_MAX_PAYLOAD_BYTES must be a positive integer")
		}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Port < 1 || c.Port > 65_535 {
		return fmt.Errorf("MCP_PORT must be between 1 and 65535")
	}
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("MCP_HOST must not be empty")
	}
	if c.ActionTimeout <= 0 || c.NavigationTimeout <= 0 {
		return fmt.Errorf("browser timeouts must be positive")
	}
	if c.SessionIdleTimeout < 0 {
		return fmt.Errorf("MCP_SESSION_IDLE_TIMEOUT_MS must not be negative")
	}
	if c.OAuthDiscoveryTimeout < time.Second || c.OAuthDiscoveryTimeout > time.Minute {
		return fmt.Errorf("MCP_OAUTH_DISCOVERY_TIMEOUT_MS must be between 1000 and 60000")
	}
	if c.SnapshotMaxChars < 1 || c.SnapshotMaxElements < 1 {
		return fmt.Errorf("snapshot limits must be positive")
	}
	if c.ObscuraMaxPayload < 1 {
		return fmt.Errorf("OBSCURA_MAX_PAYLOAD_BYTES must be positive")
	}
	if c.ObscuraFailureThreshold < 1 || c.ObscuraFailureThreshold > 20 {
		return fmt.Errorf("OBSCURA_FAILURE_THRESHOLD must be between 1 and 20")
	}
	if c.ObscuraCooldown < time.Second || c.ObscuraCooldown > 10*time.Minute {
		return fmt.Errorf("OBSCURA_COOLDOWN_MS must be between 1000 and 600000")
	}
	if c.ObscuraEndpoint != "" {
		if err := validateHTTPURL("OBSCURA_MCP_ENDPOINT", c.ObscuraEndpoint); err != nil {
			return err
		}
	}
	if err := validateHTTPURL("MCP_CDP_ENDPOINT", c.CDPEndpoint); err != nil {
		return err
	}
	if err := validateHTTPURL("HUMAN_TAKEOVER_URL", c.HumanTakeoverURL); err != nil {
		return err
	}
	if c.OAuthEnabled {
		if c.APIKey != "" {
			return fmt.Errorf("MCP_API_KEY and MCP_OAUTH_ENABLED cannot be used together")
		}
		if c.PublicURL == "" {
			return fmt.Errorf("MCP_PUBLIC_URL is required when MCP_OAUTH_ENABLED is true")
		}
		if err := validateHTTPSURL("MCP_PUBLIC_URL", c.PublicURL); err != nil {
			return err
		}
		publicURL, _ := url.Parse(c.PublicURL)
		if publicURL.Path != "/mcp" {
			return fmt.Errorf("MCP_PUBLIC_URL must end with the exact /mcp endpoint")
		}
		if c.OAuthIssuer == "" {
			return fmt.Errorf("MCP_OAUTH_ISSUER is required when MCP_OAUTH_ENABLED is true")
		}
		if err := validateHTTPSURL("MCP_OAUTH_ISSUER", c.OAuthIssuer); err != nil {
			return err
		}
		if c.OAuthAudience == "" {
			return fmt.Errorf("MCP_OAUTH_AUDIENCE is required when MCP_OAUTH_ENABLED is true")
		}
		if c.OAuthAudience != c.PublicURL {
			return fmt.Errorf("MCP_OAUTH_AUDIENCE must exactly match MCP_PUBLIC_URL")
		}
		if len(c.OAuthAllowedSubjects) == 0 {
			return fmt.Errorf("MCP_OAUTH_ALLOWED_SUBJECTS must contain at least one subject when MCP_OAUTH_ENABLED is true")
		}
	}
	return nil
}

func csvValue(lookup func(string) (string, bool), name, fallback string) []string {
	raw := fallback
	if value, ok := lookup(name); ok {
		raw = value
	}
	values := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func (c Config) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

func validateHTTPURL(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", name, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", name)
	}
	if u.Host == "" {
		return fmt.Errorf("%s must include a host", name)
	}
	if u.User != nil {
		return fmt.Errorf("%s must not contain credentials", name)
	}
	return nil
}

func validateHTTPSURL(name, raw string) error {
	if err := validateHTTPURL(name, raw); err != nil {
		return err
	}
	u, _ := url.Parse(raw)
	if u.Scheme != "https" {
		return fmt.Errorf("%s must use https", name)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%s must not contain a query or fragment", name)
	}
	return nil
}

func stringValue(lookup func(string) (string, bool), name, fallback string) string {
	if raw, ok := lookup(name); ok && strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	return fallback
}

func intValue(lookup func(string) (string, bool), name string, fallback int) (int, error) {
	raw, ok := lookup(name)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return value, nil
}

func boolValue(lookup func(string) (string, bool), name string, fallback bool) (bool, error) {
	raw, ok := lookup(name)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return value, nil
}

func durationMS(lookup func(string) (string, bool), name string, fallback time.Duration) (time.Duration, error) {
	value, err := intValue(lookup, name, int(fallback/time.Millisecond))
	if err != nil {
		return 0, err
	}
	return time.Duration(value) * time.Millisecond, nil
}
