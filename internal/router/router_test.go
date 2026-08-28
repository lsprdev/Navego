package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lsprdev/Navego/internal/browser"
	"github.com/lsprdev/Navego/internal/obscura"
)

type fakeChromium struct {
	opened []string
}

func (f *fakeChromium) Status(context.Context) (browser.Status, error) {
	return browser.Status{Connected: true, URL: "https://chromium.example", Backend: "chromium"}, nil
}
func (f *fakeChromium) Open(_ context.Context, rawURL string) (browser.Snapshot, error) {
	f.opened = append(f.opened, rawURL)
	return browser.Snapshot{URL: rawURL, Backend: "chromium", Generation: 1}, nil
}
func (f *fakeChromium) Snapshot(context.Context) (browser.Snapshot, error) {
	return browser.Snapshot{URL: "https://chromium.example", Backend: "chromium"}, nil
}
func (f *fakeChromium) Click(context.Context, string) (browser.Snapshot, error) {
	return browser.Snapshot{Backend: "chromium"}, nil
}
func (f *fakeChromium) Type(context.Context, string, string, bool) (browser.Snapshot, error) {
	return browser.Snapshot{Backend: "chromium"}, nil
}
func (f *fakeChromium) Screenshot(context.Context, bool) ([]byte, string, error) {
	return []byte("chromium-png"), "image/png", nil
}
func (f *fakeChromium) PDF(context.Context) ([]byte, string, error) {
	return []byte("chromium-pdf"), "application/pdf", nil
}
func (f *fakeChromium) DescribeAction(context.Context, string) (browser.ActionTarget, error) {
	return browser.ActionTarget{}, nil
}
func (f *fakeChromium) CommitAction(context.Context, browser.ActionTarget) (browser.Snapshot, error) {
	return browser.Snapshot{Backend: "chromium"}, nil
}
func (f *fakeChromium) Close() error { return nil }

type fakePublic struct {
	openErr  error
	opened   []string
	attempts int
}

func (f *fakePublic) Open(_ context.Context, rawURL, _ string, _ int) (obscura.Snapshot, error) {
	f.attempts++
	if f.openErr != nil {
		return obscura.Snapshot{}, f.openErr
	}
	f.opened = append(f.opened, rawURL)
	return obscura.Snapshot{URL: rawURL, Title: "Public", Text: "Body"}, nil
}
func (f *fakePublic) Snapshot(context.Context, int) (obscura.Snapshot, error) {
	return obscura.Snapshot{URL: "https://example.com", Title: "Public", Text: "Body"}, nil
}
func (f *fakePublic) Markdown(context.Context, int) (string, error) {
	return "# Public", nil
}
func (f *fakePublic) Links(context.Context, bool, int) ([]obscura.Link, error) {
	return []obscura.Link{{Text: "Public story", Href: "https://example.com/story"}}, nil
}
func (f *fakePublic) Screenshot(context.Context, int, int) ([]byte, string, error) {
	return []byte("obscura-png"), "image/png", nil
}
func (f *fakePublic) PDF(context.Context) ([]byte, string, error) {
	return []byte("obscura-pdf"), "application/pdf", nil
}
func (f *fakePublic) Close() error { return nil }

type fakeMetadata struct {
	value browser.Metadata
}

func (f fakeMetadata) Fetch(context.Context, string) (browser.Metadata, error) {
	return f.value, nil
}

func TestRoutesPublicReadsToObscura(t *testing.T) {
	t.Parallel()
	chromium := &fakeChromium{}
	public := &fakePublic{}
	router := New(chromium, public, []string{"x.com"}, 4_000, nil)

	snapshot, err := router.Open(t.Context(), "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Backend != "obscura" || len(public.opened) != 1 || len(chromium.opened) != 0 {
		t.Fatalf("unexpected route: snapshot=%+v public=%v chromium=%v", snapshot, public.opened, chromium.opened)
	}
	status, err := router.Status(t.Context())
	if err != nil || status.Backend != "obscura" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	image, _, err := router.Screenshot(t.Context(), false)
	if err != nil || string(image) != "obscura-png" {
		t.Fatalf("image=%q err=%v", image, err)
	}
	if _, err := router.Click(t.Context(), "e1"); err == nil {
		t.Fatal("Obscura interaction should require a Chromium handoff")
	}
}

func TestAlwaysChromiumAndHumanHandoff(t *testing.T) {
	t.Parallel()
	chromium := &fakeChromium{}
	public := &fakePublic{}
	router := New(chromium, public, []string{"x.com"}, 4_000, nil)

	if _, err := router.Open(t.Context(), "https://mobile.x.com/home"); err != nil {
		t.Fatal(err)
	}
	if len(chromium.opened) != 1 || len(public.opened) != 0 {
		t.Fatalf("x.com should route to Chromium: public=%v chromium=%v", public.opened, chromium.opened)
	}

	if _, err := router.Open(t.Context(), "https://portal.example.edu/login"); err != nil {
		t.Fatal(err)
	}
	if err := router.PrepareHumanTakeover(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(chromium.opened) != 2 || chromium.opened[1] != "https://portal.example.edu/login" {
		t.Fatalf("handoff opens=%v", chromium.opened)
	}
	status, _ := router.Status(t.Context())
	if status.Backend != "chromium" {
		t.Fatalf("handoff status=%+v", status)
	}
}

func TestFallsBackToChromiumWhenObscuraFails(t *testing.T) {
	t.Parallel()
	chromium := &fakeChromium{}
	public := &fakePublic{openErr: errors.New("unsupported page")}
	router := New(chromium, public, nil, 4_000, nil)

	snapshot, err := router.Open(t.Context(), "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Backend != "chromium" || len(chromium.opened) != 1 {
		t.Fatalf("fallback snapshot=%+v opens=%v", snapshot, chromium.opened)
	}
}

func TestPublicMetadataFindAndWait(t *testing.T) {
	t.Parallel()
	chromium := &fakeChromium{}
	public := &fakePublic{}
	router := New(chromium, public, nil, 4_000, nil, WithMetadataFetcher(fakeMetadata{value: browser.Metadata{
		ImageURL:       "https://example.com/story.png",
		ArticleSection: "News",
	}}))

	snapshot, err := router.Open(t.Context(), "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Metadata.ImageURL != "https://example.com/story.png" || snapshot.Metadata.ArticleSection != "News" {
		t.Fatalf("metadata=%+v", snapshot.Metadata)
	}
	found, err := router.Find(t.Context(), "Public", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Matches) == 0 || found.Matches[0].Href != "https://example.com/story" {
		t.Fatalf("find=%+v", found)
	}
	waited, err := router.Wait(t.Context(), browser.WaitCondition{Text: "Body"})
	if err != nil || waited.Backend != "obscura" {
		t.Fatalf("waited=%+v err=%v", waited, err)
	}
}

func TestCircuitBreakerSkipsAndRecovers(t *testing.T) {
	t.Parallel()
	chromium := &fakeChromium{}
	public := &fakePublic{openErr: errors.New("offline")}
	router := New(chromium, public, nil, 4_000, nil, WithCircuitBreaker(3, time.Second))
	now := time.Unix(1_800_000_000, 0)
	router.now = func() time.Time { return now }

	for range 4 {
		if _, err := router.Open(t.Context(), "https://example.com"); err != nil {
			t.Fatal(err)
		}
	}
	if public.attempts != 3 {
		t.Fatalf("Obscura attempts=%d; want 3 before open circuit", public.attempts)
	}
	status, err := router.Status(t.Context())
	if err != nil || status.Routing == nil || status.Routing.CircuitState != "open" {
		t.Fatalf("status=%+v err=%v", status, err)
	}

	now = now.Add(2 * time.Second)
	public.openErr = nil
	snapshot, err := router.Open(t.Context(), "https://example.com/recovered")
	if err != nil || snapshot.Backend != "obscura" || public.attempts != 4 {
		t.Fatalf("snapshot=%+v attempts=%d err=%v", snapshot, public.attempts, err)
	}
	status, _ = router.Status(t.Context())
	if status.Routing.CircuitState != "closed" || status.Routing.ConsecutiveFailure != 0 {
		t.Fatalf("recovered status=%+v", status)
	}
}
