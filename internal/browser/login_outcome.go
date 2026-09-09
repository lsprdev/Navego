package browser

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var browserPageErrorCode = regexp.MustCompile(`\bERR_[A-Z0-9_]{1,80}\b`)

// SavedLoginPageError rejects known browser error pages, but deliberately does
// not claim a normal document proves authentication (MFA and rejected passwords
// can also return normal documents).
func SavedLoginPageError(snapshot Snapshot) error {
	u, err := url.Parse(snapshot.URL)
	if err != nil {
		return fmt.Errorf("saved login result has an invalid page URL; authentication is not confirmed")
	}
	if u.Scheme != "chrome-error" && !(u.Scheme == "chrome" && u.Host == "network-error") {
		return nil
	}
	code := browserPageErrorCode.FindString(strings.ToUpper(snapshot.Text))
	if code == "" {
		code = "CHROME_ERROR_PAGE"
	}
	// Only a constrained code is returned, never the document's text or URL.
	return fmt.Errorf("saved login reached a browser error page: %s; authentication is not confirmed; inspect worker logs for browser request blocked", code)
}
