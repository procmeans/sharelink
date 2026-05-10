package douyin

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/procmeans/sharelink/internal/httpclient"
)

var videoIDRe = regexp.MustCompile(`(?:share/video|video)/(\d+)`)

// ResolveVideoID GETs shortURL with a no-redirect client, reads the Location
// header from the 3xx response, and extracts the numeric video id.
func ResolveVideoID(client *http.Client, shortURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, shortURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", httpclient.MobileUA)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: do request: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("%w: unexpected status %d", ErrUpstream, resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("%w: missing Location header", ErrInvalidRedirect)
	}

	m := videoIDRe.FindStringSubmatch(loc)
	if len(m) < 2 {
		return "", fmt.Errorf("%w: location %q", ErrInvalidRedirect, loc)
	}
	return m[1], nil
}
