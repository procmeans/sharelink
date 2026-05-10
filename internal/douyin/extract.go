package douyin

import (
	"github.com/procmeans/sharelink/internal/httpclient"
)

// Config lets tests inject a fake short URL and share base. Production code
// should call Extract(input) instead.
type Config struct {
	// ShortURL is the v.douyin.com short URL to resolve.
	ShortURL string
	// ShareBaseURL is the base URL for the share page (default: shareBaseURL).
	ShareBaseURL string
}

// Extract is the package's high-level entry point: take user-pasted share
// text or a short URL and return a watermark-free mp4 URL.
func Extract(input string) (string, error) {
	short, err := ExtractShortURL(input)
	if err != nil {
		return "", err
	}
	return ExtractWithConfig(Config{ShortURL: short})
}

// ExtractWithConfig runs the resolve→fetch→strip pipeline with an explicit
// short URL and (optional) share base. Used by Extract and tests.
func ExtractWithConfig(cfg Config) (string, error) {
	if cfg.ShareBaseURL == "" {
		cfg.ShareBaseURL = shareBaseURL
	}

	videoID, err := ResolveVideoID(httpclient.NewNoRedirect(), cfg.ShortURL)
	if err != nil {
		return "", err
	}

	playWM, err := fetchPlayURLFromBase(httpclient.NewFollowing(), cfg.ShareBaseURL, videoID)
	if err != nil {
		return "", err
	}

	return StripWatermark(playWM), nil
}
