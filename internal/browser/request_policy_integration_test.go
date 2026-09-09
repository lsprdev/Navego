//go:build integration

package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestBlockedNavigationBecomesLoginPageError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.UserDataDir(t.TempDir()))
	if executable := os.Getenv("NAVEGO_TEST_CHROME"); executable != "" {
		opts = append(opts, chromedp.ExecPath(executable))
	}
	allocator, stopAllocator := chromedp.NewExecAllocator(ctx, opts...)
	defer stopAllocator()
	tab, stopTab := chromedp.NewContext(allocator)
	defer stopTab()
	if err := chromedp.Run(tab, chromedp.Navigate("about:blank")); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(ctx, "", 5*time.Second, 15*time.Second, 12000, 150)
	manager.browserContext = tab
	if err := manager.enableRequestPolicyLocked(tab); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	err := chromedp.Run(tab, chromedp.Navigate(server.URL+"/login"))
	if err == nil || !strings.Contains(err.Error(), "ERR_BLOCKED_BY_CLIENT") {
		t.Fatalf("navigation not blocked: %v", err)
	}
	if err := chromedp.Run(tab, chromedp.Sleep(200*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.snapshotLocked(tab)
	if err != nil {
		t.Fatal(err)
	}
	if err := SavedLoginPageError(snapshot); err == nil {
		t.Fatalf("Chrome error page accepted: %+v", snapshot)
	}
	if requests.Load() != 0 {
		t.Fatal("blocked request reached the local server")
	}
}
