package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/procmeans/sharelink/internal/httpclient"
)

// download GETs url with headers and writes the body to path. A partial file
// is removed on failure.
func download(url string, headers map[string]string, path string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	req.Header.Set("User-Agent", httpclient.MobileUA)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download: unexpected status %d", resp.StatusCode)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(path)
		return fmt.Errorf("download: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return fmt.Errorf("download: %w", err)
	}
	return nil
}
