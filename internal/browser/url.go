package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultDNSCacheTTL = 5 * time.Second

var blockedPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
}

type ipResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type cachedHost struct {
	expiresAt time.Time
}

type pendingHost struct {
	done chan struct{}
	err  error
}

// PolicyError carries a safe diagnostic code; callers should not log raw URLs.
type PolicyError struct {
	Code string
	err  error
}

func (e *PolicyError) Error() string { return e.err.Error() }
func (e *PolicyError) Unwrap() error { return e.err }

func dnsPolicyError(host string, err error) error {
	code := "dns_error"
	var dnsErr *net.DNSError
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &dnsErr) && dnsErr.Timeout()) {
		code = "dns_timeout"
	} else if errors.Is(err, context.Canceled) {
		code = "canceled"
	}
	return &PolicyError{Code: code, err: fmt.Errorf("resolve %s: %w", host, err)}
}

// PublicURLPolicy validates both the URL syntax and the current DNS answers.
// A short allow cache avoids resolving every asset while limiting the useful
// lifetime of a DNS rebinding answer.
type PublicURLPolicy struct {
	resolver ipResolver
	cacheTTL time.Duration

	mu      sync.Mutex
	cache   map[string]cachedHost
	pending map[string]*pendingHost
}

func NewPublicURLPolicy() *PublicURLPolicy {
	return newPublicURLPolicy(net.DefaultResolver, defaultDNSCacheTTL)
}

func newPublicURLPolicy(resolver ipResolver, cacheTTL time.Duration) *PublicURLPolicy {
	return &PublicURLPolicy{
		resolver: resolver,
		cacheTTL: cacheTTL,
		cache:    make(map[string]cachedHost),
		pending:  make(map[string]*pendingHost),
	}
}

func ValidatePublicURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("URL must use http or https")
	}
	if u.Hostname() == "" || u.User != nil {
		return nil, fmt.Errorf("URL must include a host and no credentials")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") ||
		strings.HasSuffix(host, ".lan") || strings.HasSuffix(host, ".home.arpa") {
		return nil, fmt.Errorf("local URLs are blocked")
	}
	if !strings.Contains(host, ".") && net.ParseIP(host) == nil {
		return nil, fmt.Errorf("single-label hostnames are blocked")
	}
	if ip := net.ParseIP(host); ip != nil && blockedIP(ip) {
		return nil, fmt.Errorf("private or local IP addresses are blocked")
	}
	return u, nil
}

func (p *PublicURLPolicy) Validate(ctx context.Context, raw string) (*url.URL, error) {
	u, err := ValidatePublicURL(raw)
	if err != nil {
		return nil, &PolicyError{Code: "url_rejected", err: err}
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if net.ParseIP(host) != nil {
		return u, nil
	}

	now := time.Now()
	p.mu.Lock()
	cached, ok := p.cache[host]
	if ok && now.Before(cached.expiresAt) {
		p.mu.Unlock()
		return u, nil
	}
	delete(p.cache, host)
	if pending := p.pending[host]; pending != nil {
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, dnsPolicyError(host, ctx.Err())
		case <-pending.done:
			if pending.err != nil {
				return nil, pending.err
			}
			return u, nil
		}
	}
	pending := &pendingHost{done: make(chan struct{})}
	p.pending[host] = pending
	p.mu.Unlock()

	err = p.validateHost(ctx, host)
	p.mu.Lock()
	if err == nil {
		// Start the short TTL only after resolution has completed. Slow DNS
		// must not consume the entire cache lifetime before it is populated.
		p.cache[host] = cachedHost{expiresAt: time.Now().Add(p.cacheTTL)}
	}
	pending.err = err
	delete(p.pending, host)
	close(pending.done)
	p.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (p *PublicURLPolicy) validateHost(ctx context.Context, host string) error {
	addresses, err := p.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return dnsPolicyError(host, err)
	}
	if len(addresses) == 0 {
		return &PolicyError{Code: "dns_empty", err: fmt.Errorf("resolve %s: no addresses returned", host)}
	}
	for _, address := range addresses {
		if blockedIP(address.IP) {
			return &PolicyError{Code: "non_public_ip", err: fmt.Errorf("%s resolves to a private, local, or reserved address", host)}
		}
	}

	return nil
}

func blockedIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() ||
		address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() ||
		address.IsUnspecified() {
		return true
	}
	for _, prefix := range blockedPublicPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// IsPublicIP reports whether an address is safe for an outbound public-web
// connection. Callers that implement their own dialer use this to pin the
// validated address and avoid a second, unchecked DNS lookup.
func IsPublicIP(ip net.IP) bool {
	return !blockedIP(ip)
}
