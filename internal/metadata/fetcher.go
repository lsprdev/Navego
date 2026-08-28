package metadata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lsprdev/Navego/internal/browser"
	"golang.org/x/net/html"
)

const (
	defaultMaxHTML   = 2 << 20
	defaultTimeout   = 8 * time.Second
	maxRedirectCount = 5
)

type Fetcher struct {
	client    *http.Client
	urlPolicy *browser.PublicURLPolicy
	maxHTML   int64
	userAgent string
}

func New() *Fetcher {
	policy := browser.NewPublicURLPolicy()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = publicDialContext
	transport.MaxIdleConns = 10
	transport.MaxIdleConnsPerHost = 2
	transport.IdleConnTimeout = 30 * time.Second
	client := &http.Client{
		Timeout:   defaultTimeout,
		Transport: transport,
	}
	fetcher := &Fetcher{
		client:    client,
		urlPolicy: policy,
		maxHTML:   defaultMaxHTML,
		userAgent: "navego-metadata/0.1",
	}
	client.CheckRedirect = fetcher.checkRedirect
	return fetcher
}

func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (browser.Metadata, error) {
	response, err := f.openHTML(ctx, rawURL)
	if err != nil {
		return browser.Metadata{}, err
	}
	defer response.Body.Close()

	limited := io.LimitReader(response.Body, f.maxHTML+1)
	metadata, readBytes, err := parse(limited, response.Request.URL)
	if err != nil {
		return browser.Metadata{}, err
	}
	if readBytes > f.maxHTML {
		return browser.Metadata{}, errors.New("fetch page metadata: HTML exceeded the size limit")
	}
	if metadata.ImageURL != "" {
		image, err := f.urlPolicy.Validate(ctx, metadata.ImageURL)
		if err != nil {
			metadata.ImageURL = ""
			metadata.ImageAlt = ""
		} else {
			metadata.ImageURL = image.String()
		}
	}
	return metadata, nil
}

func (f *Fetcher) Links(ctx context.Context, rawURL string, limit int) ([]browser.PageLink, error) {
	if limit < 1 || limit > 500 {
		return nil, errors.New("HTML link limit must be between 1 and 500")
	}
	response, err := f.openHTML(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	links, readBytes, err := parseLinks(io.LimitReader(response.Body, f.maxHTML+1), response.Request.URL, limit)
	if err != nil {
		return nil, err
	}
	if readBytes > f.maxHTML {
		return nil, errors.New("fetch page links: HTML exceeded the size limit")
	}
	return links, nil
}

func (f *Fetcher) openHTML(ctx context.Context, rawURL string) (*http.Response, error) {
	u, err := f.urlPolicy.Validate(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create public HTML request: %w", err)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("User-Agent", f.userAgent)
	response, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch public HTML: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		response.Body.Close()
		return nil, fmt.Errorf("fetch public HTML: unexpected HTTP status %d", response.StatusCode)
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "text/html") && !strings.Contains(contentType, "application/xhtml+xml") {
		response.Body.Close()
		return nil, fmt.Errorf("fetch public HTML: unsupported content type %q", contentType)
	}
	return response, nil
}

func (f *Fetcher) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirectCount {
		return errors.New("fetch page metadata: too many redirects")
	}
	ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
	defer cancel()
	_, err := f.urlPolicy.Validate(ctx, req.URL.String())
	return err
}

func publicDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("parse outbound address: %w", err)
	}
	addresses := []net.IPAddr{}
	if ip := net.ParseIP(host); ip != nil {
		addresses = append(addresses, net.IPAddr{IP: ip})
	} else {
		addresses, err = net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve outbound host %s: %w", host, err)
		}
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("resolve outbound host %s: no addresses returned", host)
	}
	for _, address := range addresses {
		if !browser.IsPublicIP(address.IP) {
			return nil, fmt.Errorf("outbound host %s resolves to a private, local, or reserved address", host)
		}
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	var errs []error
	for _, address := range addresses {
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		errs = append(errs, dialErr)
	}
	return nil, errors.Join(errs...)
}

func parse(reader io.Reader, baseURL *url.URL) (browser.Metadata, int64, error) {
	counting := &countingReader{reader: reader}
	tokenizer := html.NewTokenizer(counting)
	values := make(map[string]string)
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case html.ErrorToken:
			if err := tokenizer.Err(); err != nil && !errors.Is(err, io.EOF) {
				return browser.Metadata{}, counting.read, fmt.Errorf("parse page metadata: %w", err)
			}
			return build(values, baseURL), counting.read, nil
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			switch strings.ToLower(token.Data) {
			case "body":
				return build(values, baseURL), counting.read, nil
			case "meta":
				key, value := metaPair(token.Attr)
				if key != "" && value != "" {
					if _, exists := values[key]; !exists {
						values[key] = value
					}
				}
			}
		}
	}
}

func parseLinks(reader io.Reader, baseURL *url.URL, limit int) ([]browser.PageLink, int64, error) {
	counting := &countingReader{reader: reader}
	tokenizer := html.NewTokenizer(counting)
	links := make([]browser.PageLink, 0, limit)
	seen := make(map[string]struct{})
	var currentHref string
	var currentText strings.Builder
	finish := func() {
		if currentHref == "" {
			currentText.Reset()
			return
		}
		text := truncate(currentText.String(), 300)
		if text == "" {
			text = currentHref
		}
		key := text + "\x00" + currentHref
		if _, exists := seen[key]; !exists && len(links) < limit {
			seen[key] = struct{}{}
			links = append(links, browser.PageLink{Text: text, Href: currentHref})
		}
		currentHref = ""
		currentText.Reset()
	}
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case html.ErrorToken:
			finish()
			if err := tokenizer.Err(); err != nil && !errors.Is(err, io.EOF) {
				return nil, counting.read, fmt.Errorf("parse page links: %w", err)
			}
			return links, counting.read, nil
		case html.StartTagToken:
			token := tokenizer.Token()
			if strings.EqualFold(token.Data, "a") {
				finish()
				for _, attribute := range token.Attr {
					if !strings.EqualFold(attribute.Key, "href") || len(attribute.Val) > 2_048 {
						continue
					}
					parsed, err := url.Parse(strings.TrimSpace(attribute.Val))
					if err != nil {
						continue
					}
					resolved := baseURL.ResolveReference(parsed)
					if validated, err := browser.ValidatePublicURL(resolved.String()); err == nil {
						currentHref = validated.String()
					}
				}
			}
		case html.TextToken:
			if currentHref != "" && currentText.Len() < 2_048 {
				currentText.WriteString(" ")
				currentText.WriteString(tokenizer.Token().Data)
			}
		case html.EndTagToken:
			if strings.EqualFold(tokenizer.Token().Data, "a") {
				finish()
				if len(links) >= limit {
					return links, counting.read, nil
				}
			}
		}
	}
}

func metaPair(attributes []html.Attribute) (string, string) {
	var key, value string
	for _, attribute := range attributes {
		switch strings.ToLower(attribute.Key) {
		case "property", "name":
			key = strings.ToLower(strings.TrimSpace(attribute.Val))
		case "content":
			value = strings.TrimSpace(attribute.Val)
		}
	}
	switch key {
	case "og:description", "description", "og:image", "twitter:image", "og:image:alt", "og:site_name", "og:type", "article:section":
		return key, value
	default:
		return "", ""
	}
}

func build(values map[string]string, baseURL *url.URL) browser.Metadata {
	imageURL := first(values["og:image"], values["twitter:image"])
	if imageURL != "" {
		if parsed, err := url.Parse(imageURL); err == nil {
			imageURL = baseURL.ResolveReference(parsed).String()
		} else {
			imageURL = ""
		}
	}
	return browser.Metadata{
		Description:    truncate(first(values["og:description"], values["description"]), 500),
		ImageURL:       imageURL,
		ImageAlt:       truncate(values["og:image:alt"], 300),
		SiteName:       truncate(values["og:site_name"], 200),
		Type:           truncate(values["og:type"], 100),
		ArticleSection: truncate(values["article:section"], 200),
	}
}

func first(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func truncate(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}

type countingReader struct {
	reader io.Reader
	read   int64
}

func (r *countingReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	r.read += int64(n)
	return n, err
}
