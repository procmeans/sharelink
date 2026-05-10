package douyin

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/procmeans/sharelink/internal/httpclient"
)

const ttwidSourceURL = "https://www.douyin.com/"

// TTWidManager fetches and stores the douyin "ttwid" cookie.
type TTWidManager struct {
	client *http.Client
	url    string

	mu    sync.RWMutex
	value string
}

// NewTTWidManager constructs a manager that fetches ttwid from douyin.com.
func NewTTWidManager(client *http.Client) *TTWidManager {
	return newTTWidManagerWithURL(client, ttwidSourceURL)
}

// newTTWidManagerWithURL is the test seam.
func newTTWidManagerWithURL(client *http.Client, url string) *TTWidManager {
	return &TTWidManager{client: client, url: url}
}

// withURLForTest returns the same manager with the source URL replaced.
// Used only by tests that need to point an existing manager at a different host.
func (m *TTWidManager) withURLForTest(url string) *TTWidManager {
	m.mu.Lock()
	m.url = url
	m.mu.Unlock()
	return m
}

// Cookie returns the current ttwid value, or "" if none has been acquired.
func (m *TTWidManager) Cookie() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.value
}

// Refresh fetches a fresh ttwid from the source URL. The HTTP request is made
// without holding the write lock, so concurrent Cookie() reads are not blocked.
// On failure the previously-stored value is preserved.
func (m *TTWidManager) Refresh() error {
	m.mu.RLock()
	url := m.url
	m.mu.RUnlock()

	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return fmt.Errorf("%w: build ttwid request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", httpclient.MobileUA)

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: do ttwid request: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("%w: ttwid status %d", ErrUpstream, resp.StatusCode)
	}

	var found string
	for _, ck := range resp.Cookies() {
		if ck.Name == "ttwid" {
			found = ck.Value
			break
		}
	}
	if found == "" {
		return fmt.Errorf("%w: ttwid cookie not in Set-Cookie", ErrUpstream)
	}

	m.mu.Lock()
	m.value = found
	m.mu.Unlock()
	return nil
}
