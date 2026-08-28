package metadata

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestParseOpenGraphMetadata(t *testing.T) {
	base := mustURL(t, "https://example.com/news/story")
	input := `<html><head>
		<meta property="og:description" content=" Story description ">
		<meta property="og:image" content="/images/story.png">
		<meta property="og:image:alt" content="Story image">
		<meta property="og:site_name" content="Example News">
		<meta property="og:type" content="article">
		<meta property="article:section" content="Politics">
	</head><body><meta property="og:image" content="https://attacker.example/late.png"></body></html>`
	metadata, _, err := parse(strings.NewReader(input), base)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ImageURL != "https://example.com/images/story.png" || metadata.ArticleSection != "Politics" {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
	if metadata.Description != "Story description" || metadata.SiteName != "Example News" || metadata.Type != "article" {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
}

func TestMetaPairIgnoresUnrelatedMetadata(t *testing.T) {
	key, value := metaPair(nil)
	if key != "" || value != "" {
		t.Fatalf("empty attributes returned %q %q", key, value)
	}
}

func TestPublicDialContextRejectsLoopback(t *testing.T) {
	if _, err := publicDialContext(context.Background(), "tcp", "127.0.0.1:80"); err == nil {
		t.Fatal("expected loopback address to be rejected before dialing")
	}
}

func TestParseLinksResolvesAndFiltersURLs(t *testing.T) {
	input := `<html><body>
		<a href="/politica/story"><span>Story</span> title</a>
		<a href="javascript:alert(1)">Unsafe</a>
		<a href="http://127.0.0.1/private">Private</a>
	</body></html>`
	links, _, err := parseLinks(strings.NewReader(input), mustURL(t, "https://example.com/news"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].Href != "https://example.com/politica/story" || links[0].Text != "Story title" {
		t.Fatalf("links=%+v", links)
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
