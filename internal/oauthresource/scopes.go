package oauthresource

import (
	"net/url"
	"strings"
)

const (
	ScopeRead     = "browser:read"
	ScopeCapture  = "browser:capture"
	ScopeInteract = "browser:interact"
	ScopeWrite    = "browser:write"
	ScopeTakeover = "browser:takeover"
)

var AllScopes = []string{
	ScopeRead,
	ScopeCapture,
	ScopeInteract,
	ScopeWrite,
	ScopeTakeover,
}

// ProtectedResourceMetadataLocations returns the canonical discovery URL used
// in challenges and both RFC 9728 locations that an MCP client may probe.
func ProtectedResourceMetadataLocations(publicURL string) (string, []string) {
	u, _ := url.Parse(publicURL)
	rootPath := "/.well-known/oauth-protected-resource"
	paths := []string{rootPath}
	if resourcePath := strings.TrimSuffix(u.EscapedPath(), "/"); resourcePath != "" {
		paths = append(paths, rootPath+resourcePath)
	}
	metadataURL := u.Scheme + "://" + u.Host + rootPath
	return metadataURL, paths
}
