// Package extract routes user input to the Douyin or TikTok extractor and
// returns a platform-neutral Result.
package extract

import (
	"github.com/procmeans/sharelink/internal/douyin"
	"github.com/procmeans/sharelink/internal/tiktok"
)

// Result is the platform-neutral extraction result.
type Result struct {
	PlayURL string
	VideoID string
	// Headers, if non-empty, must be sent when fetching PlayURL (TikTok).
	Headers        map[string]string
	CacheHit       bool
	TTWidRefreshed bool
}

// Router dispatches by link type. Fields are injectable for tests.
type Router struct {
	Douyin func(input string) (douyin.Result, error)
	TikTok func(input string) (tiktok.Result, error)
}

// Extract sends input containing a TikTok link to the TikTok extractor and
// everything else to Douyin (which reports ErrNoShortURL when no link is found).
func (r Router) Extract(input string) (Result, error) {
	if _, err := tiktok.ExtractURL(input); err == nil {
		res, err := r.TikTok(input)
		return Result{PlayURL: res.PlayURL, VideoID: res.VideoID, Headers: res.Headers}, err
	}
	res, err := r.Douyin(input)
	return Result{
		PlayURL:        res.PlayURL,
		VideoID:        res.VideoID,
		CacheHit:       res.CacheHit,
		TTWidRefreshed: res.TTWidRefreshed,
	}, err
}
