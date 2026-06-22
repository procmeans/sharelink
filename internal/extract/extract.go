// Package extract is the platform-neutral layer in front of the per-platform
// providers. It defines a common Result type and a Router that inspects share
// input and dispatches to the matching provider (Douyin or YouTube).
package extract

import (
	"errors"

	"github.com/procmeans/sharelink/internal/douyin"
	"github.com/procmeans/sharelink/internal/youtube"
)

// ErrUnsupportedURL means the input contained no link recognized by any provider.
var ErrUnsupportedURL = errors.New("extract: no supported video link found in input")

// Result is the platform-neutral extraction result.
//
// Douyin sets PlayURL (a single watermark-free mp4). YouTube high quality is
// delivered as separated DASH streams, so it sets VideoURL and AudioURL
// instead, leaving PlayURL empty.
type Result struct {
	Source         string // "douyin" or "youtube"
	VideoID        string
	Title          string
	PlayURL        string // single muxed/playable URL (Douyin)
	VideoURL       string // video-only stream (YouTube, highest quality)
	AudioURL       string // audio-only stream (YouTube)
	CacheHit       bool
	TTWidRefreshed bool
}

// Router dispatches share input to the matching platform extractor. A nil
// provider is treated as unconfigured and skipped.
type Router struct {
	douyin  *douyin.Service
	youtube *youtube.Service
}

// NewRouter wires the configured providers. Either may be nil.
func NewRouter(dy *douyin.Service, yt *youtube.Service) *Router {
	return &Router{douyin: dy, youtube: yt}
}

// Extract detects the platform from input and returns a neutral Result.
func (r *Router) Extract(input string) (Result, error) {
	switch {
	case r.youtube != nil && youtube.Matches(input):
		res, err := r.youtube.Extract(input)
		if err != nil {
			return Result{Source: "youtube"}, err
		}
		return Result{
			Source:   "youtube",
			VideoID:  res.VideoID,
			Title:    res.Title,
			PlayURL:  res.PlayURL,
			VideoURL: res.VideoURL,
			AudioURL: res.AudioURL,
		}, nil

	case r.douyin != nil && douyin.Matches(input):
		res, err := r.douyin.Extract(input)
		return Result{
			Source:         "douyin",
			VideoID:        res.VideoID,
			PlayURL:        res.PlayURL,
			CacheHit:       res.CacheHit,
			TTWidRefreshed: res.TTWidRefreshed,
		}, err

	default:
		return Result{}, ErrUnsupportedURL
	}
}
