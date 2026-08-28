package credentials

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxManifestBytes = 64 << 10
	maxSecretBytes   = 16 << 10
)

type Descriptor struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Origin string `json:"origin"`
}

type Secret struct {
	Username []byte `json:"-"`
	Password []byte `json:"-"`
}

func (s *Secret) Clear() {
	clear(s.Username)
	clear(s.Password)
	s.Username = nil
	s.Password = nil
}

type entry struct {
	descriptor   Descriptor
	usernameFile string
	passwordFile string
}

type Store struct {
	secretsRoot string
	byID        map[string]entry
	byOrigin    map[string]entry
}

type manifest struct {
	Version int             `json:"version"`
	Logins  []manifestLogin `json:"logins"`
}

type manifestLogin struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Origin       string `json:"origin"`
	UsernameFile string `json:"username_file"`
	PasswordFile string `json:"password_file"`
}

func Disabled() *Store {
	return &Store{byID: make(map[string]entry), byOrigin: make(map[string]entry)}
}

func Load(manifestPath, secretsRoot string) (*Store, error) {
	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" {
		return Disabled(), nil
	}
	if !filepath.IsAbs(manifestPath) {
		return nil, errors.New("saved-login manifest path must be absolute")
	}
	secretsRoot = filepath.Clean(strings.TrimSpace(secretsRoot))
	if !filepath.IsAbs(secretsRoot) {
		return nil, errors.New("saved-login secrets directory must be absolute")
	}
	if _, err := pathWithinRoot(secretsRoot, manifestPath); err != nil {
		return nil, fmt.Errorf("saved-login manifest: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(secretsRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve saved-login secrets directory: %w", err)
	}
	resolvedManifest, err := filepath.EvalSymlinks(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("resolve saved-login manifest: %w", err)
	}
	if _, err := pathWithinRoot(resolvedRoot, resolvedManifest); err != nil {
		return nil, errors.New("saved-login manifest resolves outside the configured secrets directory")
	}
	manifestPath = resolvedManifest

	file, err := os.Open(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("open saved-login manifest: %w", err)
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil {
		return nil, fmt.Errorf("stat saved-login manifest: %w", err)
	} else if !info.Mode().IsRegular() || info.Size() > maxManifestBytes {
		return nil, fmt.Errorf("saved-login manifest must be a regular file no larger than %d bytes", maxManifestBytes)
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxManifestBytes+1))
	decoder.DisallowUnknownFields()
	var data manifest
	if err := decoder.Decode(&data); err != nil {
		return nil, fmt.Errorf("decode saved-login manifest: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	if data.Version != 1 {
		return nil, fmt.Errorf("saved-login manifest version must be 1")
	}
	if len(data.Logins) == 0 {
		return nil, errors.New("saved-login manifest must contain at least one login")
	}

	store := &Store{
		secretsRoot: secretsRoot,
		byID:        make(map[string]entry, len(data.Logins)),
		byOrigin:    make(map[string]entry, len(data.Logins)),
	}
	for index, configured := range data.Logins {
		id := strings.TrimSpace(configured.ID)
		label := strings.TrimSpace(configured.Label)
		if id == "" || len(id) > 80 || !validID(id) {
			return nil, fmt.Errorf("saved login %d has an invalid id", index+1)
		}
		if label == "" || len(label) > 120 {
			return nil, fmt.Errorf("saved login %q has an invalid label", id)
		}
		origin, err := CanonicalOrigin(configured.Origin)
		if err != nil {
			return nil, fmt.Errorf("saved login %q origin: %w", id, err)
		}
		usernameFile, err := pathWithinRoot(secretsRoot, configured.UsernameFile)
		if err != nil {
			return nil, fmt.Errorf("saved login %q username_file: %w", id, err)
		}
		passwordFile, err := pathWithinRoot(secretsRoot, configured.PasswordFile)
		if err != nil {
			return nil, fmt.Errorf("saved login %q password_file: %w", id, err)
		}
		if usernameFile == passwordFile {
			return nil, fmt.Errorf("saved login %q must use separate username and password files", id)
		}
		if _, exists := store.byID[id]; exists {
			return nil, fmt.Errorf("saved-login id %q is duplicated", id)
		}
		if _, exists := store.byOrigin[origin]; exists {
			return nil, fmt.Errorf("only one saved login may be configured for origin %q", origin)
		}
		item := entry{
			descriptor:   Descriptor{ID: id, Label: label, Origin: origin},
			usernameFile: usernameFile,
			passwordFile: passwordFile,
		}
		store.byID[id] = item
		store.byOrigin[origin] = item
	}
	return store, nil
}

func (s *Store) Enabled() bool { return s != nil && len(s.byID) > 0 }

func (s *Store) Count() int {
	if s == nil {
		return 0
	}
	return len(s.byID)
}

func (s *Store) MatchURL(rawURL string) (Descriptor, error) {
	if s == nil || len(s.byOrigin) == 0 {
		return Descriptor{}, errors.New("no saved logins are configured; request human login instead")
	}
	origin, err := OriginFromURL(rawURL)
	if err != nil {
		return Descriptor{}, err
	}
	item, ok := s.byOrigin[origin]
	if !ok {
		return Descriptor{}, fmt.Errorf("no saved login is configured for exact origin %q; request human login instead", origin)
	}
	return item.descriptor, nil
}

func (s *Store) ReadSecret(id, rawURL string) (Secret, error) {
	if s == nil {
		return Secret{}, errors.New("no saved logins are configured")
	}
	item, ok := s.byID[strings.TrimSpace(id)]
	if !ok {
		return Secret{}, errors.New("saved login is unknown")
	}
	origin, err := OriginFromURL(rawURL)
	if err != nil {
		return Secret{}, err
	}
	if origin != item.descriptor.Origin {
		return Secret{}, errors.New("saved login does not match the current exact origin")
	}
	username, err := s.readSecretFile(item.usernameFile)
	if err != nil {
		return Secret{}, fmt.Errorf("read saved username: %w", err)
	}
	password, err := s.readSecretFile(item.passwordFile)
	if err != nil {
		clear(username)
		return Secret{}, fmt.Errorf("read saved password: %w", err)
	}
	return Secret{Username: username, Password: password}, nil
}

func (s *Store) readSecretFile(path string) ([]byte, error) {
	resolvedRoot, err := filepath.EvalSymlinks(s.secretsRoot)
	if err != nil {
		return nil, errors.New("secrets directory is unavailable")
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, errors.New("secret file is unavailable")
	}
	if _, err := pathWithinRoot(resolvedRoot, resolvedPath); err != nil {
		return nil, errors.New("secret file resolves outside the configured secrets directory")
	}
	file, err := os.Open(resolvedPath)
	if err != nil {
		return nil, errors.New("secret file is unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, errors.New("secret file metadata is unavailable")
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("secret path is not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSecretBytes+1))
	if err != nil {
		return nil, errors.New("secret file could not be read")
	}
	if len(data) > maxSecretBytes {
		clear(data)
		return nil, fmt.Errorf("secret exceeds %d bytes", maxSecretBytes)
	}
	if bytes.HasSuffix(data, []byte("\r\n")) {
		data = data[:len(data)-2]
	} else if bytes.HasSuffix(data, []byte("\n")) {
		data = data[:len(data)-1]
	}
	if len(data) == 0 {
		return nil, errors.New("secret is empty")
	}
	return data, nil
}

func CanonicalOrigin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid origin: %w", err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return "", errors.New("origin must be an absolute https URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" {
		return "", errors.New("origin must not contain credentials, path, query, or fragment")
	}
	return canonicalURLOrigin(u), nil
}

func OriginFromURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid page URL: %w", err)
	}
	if u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", errors.New("saved login requires an absolute https page URL without embedded credentials")
	}
	return canonicalURLOrigin(u), nil
}

func canonicalURLOrigin(u *url.URL) string {
	hostname := strings.ToLower(u.Hostname())
	port := u.Port()
	host := hostname
	if port != "" && port != "443" {
		host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	return "https://" + host
}

func pathWithinRoot(root, candidate string) (string, error) {
	candidate = filepath.Clean(strings.TrimSpace(candidate))
	if !filepath.IsAbs(candidate) {
		return "", errors.New("path must be absolute")
	}
	root = filepath.Clean(root)
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("path must stay inside the configured secrets directory")
	}
	if relative == "." {
		return "", errors.New("path must name a file inside the configured secrets directory")
	}
	return candidate, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("saved-login manifest must contain one JSON document")
		}
		return fmt.Errorf("decode saved-login manifest: %w", err)
	}
	return nil
}

func validID(value string) bool {
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}
