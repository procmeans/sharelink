package douyin

import (
	"github.com/procmeans/sharelink/internal/httpclient"
)

// Result is what Extract returns: the watermark-free play URL plus the
// numeric video id (useful for logging/correlation).
type Result struct {
	PlayURL string
	VideoID string
}

// Config lets tests inject a fake short URL and share base. Production code
// should call Extract(input) instead.
type Config struct {
	// ShortURL is the v.douyin.com short URL to resolve.
	ShortURL string
	// ShareBaseURL is the base URL for the share page (default: shareBaseURL).
	ShareBaseURL string
}

// Extract is the package's high-level entry point: take user-pasted share
// text or a short URL and return a watermark-free mp4 URL plus the video id.
func Extract(input string) (Result, error) {
	short, err := ExtractShortURL(input)
	if err != nil {
		return Result{}, err
	}
	return ExtractWithConfig(Config{ShortURL: short})
}

// ExtractWithConfig runs the resolve→fetch→strip pipeline with an explicit
// short URL and (optional) share base. Used by Extract and tests.
func ExtractWithConfig(cfg Config) (Result, error) {
	if cfg.ShareBaseURL == "" {
		cfg.ShareBaseURL = shareBaseURL
	}

	videoID, err := ResolveVideoID(httpclient.NewNoRedirect(), "", cfg.ShortURL)
	if err != nil {
		return Result{}, err
	}

	playWM, err := fetchPlayURLFromBase(httpclient.NewFollowing(), cfg.ShareBaseURL, videoID)
	if err != nil {
		return Result{VideoID: videoID}, err
	}

	return Result{PlayURL: StripWatermark(playWM), VideoID: videoID}, nil
}
