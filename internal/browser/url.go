package browser

import (
	"context"
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

// PublicURLPolicy validates both the URL syntax and the current DNS answers.
// A short allow cache avoids resolving every asset while limiting the useful
// lifetime of a DNS rebinding answer.
type PublicURLPolicy struct {
	resolver ipResolver
	cacheTTL time.Duration

	mu    sync.Mutex
	cache map[string]cachedHost
}

func NewPublicURLPolicy() *PublicURLPolicy {
	return newPublicURLPolicy(net.DefaultResolver, defaultDNSCacheTTL)
}

func newPublicURLPolicy(resolver ipResolver, cacheTTL time.Duration) *PublicURLPolicy {
	return &PublicURLPolicy{
		resolver: resolver,
		cacheTTL: cacheTTL,
		cache:    make(map[string]cachedHost),
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
		return nil, err
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
	p.mu.Unlock()

	addresses, err := p.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("resolve %s: no addresses returned", host)
	}
	for _, address := range addresses {
		if blockedIP(address.IP) {
			return nil, fmt.Errorf("%s resolves to a private, local, or reserved address", host)
		}
	}

	p.mu.Lock()
	p.cache[host] = cachedHost{expiresAt: now.Add(p.cacheTTL)}
	p.mu.Unlock()
	return u, nil
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
