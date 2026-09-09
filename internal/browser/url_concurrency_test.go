package browser

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type resolverFunc func(context.Context, string) ([]net.IPAddr, error)

func (f resolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return f(ctx, host)
}

func TestPolicyCoalescesConcurrentLookups(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	policy := newPublicURLPolicy(resolverFunc(func(ctx context.Context, _ string) ([]net.IPAddr, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}), time.Second)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errorsOut := make(chan error, 40)
	for range 40 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := policy.Validate(ctx, "https://example.com/asset"); errorsOut <- err }()
	}
	<-started
	// A canceled waiter must not cancel the shared resolution.
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := policy.Validate(canceled, "https://example.com/other"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	close(release)
	wg.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("DNS called %d times", calls.Load())
	}
}

func TestPolicyTTLStartsAfterSlowLookupAndRevalidates(t *testing.T) {
	var calls int
	var resolvedAt time.Time
	policy := newPublicURLPolicy(resolverFunc(func(_ context.Context, _ string) ([]net.IPAddr, error) {
		calls++
		if calls == 1 {
			resolvedAt = time.Now()
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}), time.Second)
	if _, err := policy.Validate(t.Context(), "https://example.com"); err != nil {
		t.Fatal(err)
	}
	if policy.cache["example.com"].expiresAt.Before(resolvedAt.Add(policy.cacheTTL)) {
		t.Fatal("cache lifetime started before DNS resolution completed")
	}
	if _, err := policy.Validate(t.Context(), "https://example.com/asset"); err != nil || calls != 1 {
		t.Fatalf("slow lookup expired its own cache: %v", err)
	}
	// Force expiry deterministically instead of relying on another sleep.
	policy.mu.Lock()
	policy.cache["example.com"] = cachedHost{expiresAt: time.Now().Add(-time.Second)}
	policy.mu.Unlock()
	_, err := policy.Validate(t.Context(), "https://example.com")
	var policyErr *PolicyError
	if !errors.As(err, &policyErr) || policyErr.Code != "non_public_ip" || calls != 2 {
		t.Fatalf("rebinding was not rejected: %v", err)
	}
}

func TestPolicyDoesNotCacheDNSFailures(t *testing.T) {
	calls := 0
	policy := newPublicURLPolicy(resolverFunc(func(_ context.Context, _ string) ([]net.IPAddr, error) {
		calls++
		if calls == 1 {
			return nil, &net.DNSError{IsTimeout: true, Name: "example.com"}
		}
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}), time.Second)
	_, err := policy.Validate(t.Context(), "https://example.com")
	var policyErr *PolicyError
	if !errors.As(err, &policyErr) || policyErr.Code != "dns_timeout" {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := policy.Validate(t.Context(), "https://example.com"); err != nil || calls != 2 {
		t.Fatalf("failure was cached: %v", err)
	}
}
