package browser

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestValidatePublicURL(t *testing.T) {
	for _, raw := range []string{"https://example.com", "http://x.com/home"} {
		if _, err := ValidatePublicURL(raw); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	for _, raw := range []string{
		"file:///tmp/a",
		"http://127.0.0.1",
		"http://10.0.0.1",
		"https://user:pass@example.com",
		"http://metadata.internal",
		"http://printer",
	} {
		if _, err := ValidatePublicURL(raw); err == nil {
			t.Fatalf("expected %s to be rejected", raw)
		}
	}
}

type fakeResolver struct {
	addresses map[string][]net.IPAddr
	err       error
	calls     int
}

func (r *fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return r.addresses[host], nil
}

func TestPublicURLPolicyDNS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		addresses []string
		wantError bool
	}{
		{name: "public IPv4", addresses: []string{"93.184.216.34"}},
		{name: "public IPv6", addresses: []string{"2606:2800:220:1:248:1893:25c8:1946"}},
		{name: "private", addresses: []string{"10.0.0.5"}, wantError: true},
		{name: "link local", addresses: []string{"169.254.169.254"}, wantError: true},
		{name: "carrier NAT", addresses: []string{"100.64.0.1"}, wantError: true},
		{name: "documentation", addresses: []string{"203.0.113.8"}, wantError: true},
		{name: "mixed answers", addresses: []string{"93.184.216.34", "192.168.1.2"}, wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			addresses := make([]net.IPAddr, 0, len(test.addresses))
			for _, raw := range test.addresses {
				addresses = append(addresses, net.IPAddr{IP: net.ParseIP(raw)})
			}
			resolver := &fakeResolver{addresses: map[string][]net.IPAddr{"example.com": addresses}}
			policy := newPublicURLPolicy(resolver, time.Minute)
			_, err := policy.Validate(context.Background(), "https://example.com/path")
			if (err != nil) != test.wantError {
				t.Fatalf("Validate() error = %v; wantError %v", err, test.wantError)
			}
		})
	}
}

func TestPublicURLPolicyFailsClosedAndCachesAllowedHost(t *testing.T) {
	t.Parallel()

	failing := newPublicURLPolicy(&fakeResolver{err: errors.New("DNS unavailable")}, time.Minute)
	if _, err := failing.Validate(context.Background(), "https://example.com"); err == nil {
		t.Fatal("expected DNS failure to block the URL")
	}

	resolver := &fakeResolver{addresses: map[string][]net.IPAddr{
		"example.com": {{IP: net.ParseIP("93.184.216.34")}},
	}}
	policy := newPublicURLPolicy(resolver, time.Minute)
	for range 2 {
		if _, err := policy.Validate(context.Background(), "https://example.com"); err != nil {
			t.Fatal(err)
		}
	}
	if resolver.calls != 1 {
		t.Fatalf("resolver calls = %d; want 1", resolver.calls)
	}
}
