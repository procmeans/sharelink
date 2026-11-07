// Package youtube extracts direct stream URLs from YouTube watch/share links.
//
// Unlike Douyin (whose share page embeds a ready mp4 URL), YouTube hides the
// playable URL behind a signature cipher, an n-parameter throttle and bot
// checks that change every few weeks. Rather than track those ourselves we
// shell out to yt-dlp (which must be on PATH) and read the stream URLs from
// its JSON output. High-quality YouTube is delivered as separated DASH
// streams, so Best/H264 return a video-only URL and an audio-only URL; muxing
// the two (e.g. with ffmpeg) is left to the caller.
package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrNoVideoURL means the input text contains no recognizable YouTube link.
	ErrNoVideoURL = errors.New("youtube: no youtube link found in input")

	// ErrUpstream means yt-dlp is missing, failed, or timed out.
	ErrUpstream = errors.New("youtube: upstream request failed")

	// ErrNoStream means no suitable video/audio stream was found in the response.
	ErrNoStream = errors.New("youtube: no suitable stream found")
)

// linkRe matches the common YouTube link shapes: watch?v=, youtu.be/<id>,
// shorts/<id>, embed/<id>, live/<id>. Video ids are always 11 chars.
var linkRe = regexp.MustCompile(
	`https?://(?:www\.|m\.)?(?:youtube\.com/(?:watch\?[^ ]*\bv=|shorts/|embed/|live/)|youtu\.be/)[A-Za-z0-9_-]{11}`)

// Quality selects which stream(s) Extract returns.
type Quality int

const (
	// Muxed (the default) returns a single progressive stream that already
	// carries audio and video together — directly playable, no muxing needed,
	// but capped at the resolution YouTube still ships muxed (usually 360p).
	Muxed Quality = iota
	// Best returns the highest-resolution video-only stream plus the best
	// audio-only stream, any codec (typically VP9/AV1, up to 4K+). The caller
	// muxes them (e.g. with ffmpeg).
	Best
	// H264 is like Best but restricted to H.264 video and AAC audio, so the
	// muxed result plays everywhere (QuickTime, Final Cut, mobile). Capped at
	// 1080p, since YouTube only offers VP9/AV1 above that.
	H264
)

// formatSelector maps a Quality to a yt-dlp -f expression.
func formatSelector(q Quality) string {
	switch q {
	case Best:
		return "bv+ba"
	case H264:
		return "bv[vcodec^=avc1]+ba[acodec^=mp4a]"
	default:
		return "b"
	}
}

const ytdlpTimeout = 60 * time.Second

// Result is the YouTube-specific extraction result. The router maps it into the
// platform-neutral extract.Result. Muxed quality sets PlayURL; Best/H264 set
// VideoURL and AudioURL.
type Result struct {
	VideoID  string
	Title    string
	PlayURL  string // single progressive stream (Muxed quality)
	VideoURL string // video-only stream (Best/H264 quality)
	AudioURL string // audio-only stream (Best/H264 quality)
}

// Matches reports whether input contains a recognizable YouTube link.
func Matches(input string) bool { return linkRe.MatchString(input) }

// Service runs yt-dlp at a fixed quality.
type Service struct {
	quality Quality
	// run executes yt-dlp with args and returns its stdout. Tests replace it.
	run func(args ...string) ([]byte, error)
}

// NewService returns a Service at the given quality.
func NewService(q Quality) *Service {
	return &Service{quality: q, run: runYTDLP}
}

// runYTDLP executes yt-dlp from PATH, folding its last stderr line into the
// error so failures like "Sign in to confirm you're not a bot" are visible.
func runYTDLP(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ytdlpTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errors.New("yt-dlp not found in PATH (install with: brew install yt-dlp)")
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("yt-dlp timed out after %s", ytdlpTimeout)
		}
		lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
		return nil, fmt.Errorf("yt-dlp: %v: %s", err, lines[len(lines)-1])
	}
	return stdout.Bytes(), nil
}

// ytdlpInfo is the subset of `yt-dlp -J` output we read. For a merged format
// selection ("bv+ba") the chosen streams are in RequestedFormats; for a single
// format ("b") the stream URL is at the top level.
type ytdlpInfo struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	URL              string `json:"url"`
	RequestedFormats []struct {
		URL    string `json:"url"`
		VCodec string `json:"vcodec"`
		ACodec string `json:"acodec"`
	} `json:"requested_formats"`
}

// Extract finds the first YouTube link in input and returns its stream(s)
// according to the service's quality.
func (s *Service) Extract(input string) (Result, error) {
	link := linkRe.FindString(input)
	if link == "" {
		return Result{}, ErrNoVideoURL
	}

	out, err := s.run("-J", "--no-playlist", "--no-warnings", "-f", formatSelector(s.quality), "--", link)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	var info ytdlpInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return Result{}, fmt.Errorf("%w: decode yt-dlp json: %v", ErrUpstream, err)
	}

	res := Result{VideoID: info.ID, Title: info.Title}
	if s.quality == Muxed {
		if info.URL == "" {
			return Result{}, ErrNoStream
		}
		res.PlayURL = info.URL
		return res, nil
	}

	for _, f := range info.RequestedFormats {
		switch {
		case f.VCodec != "" && f.VCodec != "none":
			res.VideoURL = f.URL
		case f.ACodec != "" && f.ACodec != "none":
			res.AudioURL = f.URL
		}
	}
	if res.VideoURL == "" || res.AudioURL == "" {
		return Result{}, ErrNoStream
	}
	return res, nil
}
