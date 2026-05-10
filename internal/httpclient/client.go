package httpclient

import (
	"net/http"
	"time"
)

// MobileUA is an iPhone Safari user-agent. iesdouyin.com returns the share
// page reliably for mobile UAs; PC UAs sometimes get a different shell.
const MobileUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) " +
	"AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1"

const defaultTimeout = 8 * time.Second

// NewFollowing returns a client that follows redirects normally.
func NewFollowing() *http.Client {
	return &http.Client{
		Timeout: defaultTimeout,
	}
}

// NewNoRedirect returns a client that does NOT follow redirects;
// the caller is expected to read Location from the response.
func NewNoRedirect() *http.Client {
	return &http.Client{
		Timeout: defaultTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
