// Package youtube extracts direct stream URLs from YouTube watch/share links.
//
// Unlike Douyin (whose share page embeds a ready mp4 URL), YouTube hides the
// playable URL behind a signature cipher and an n-parameter throttle that must
// be decoded by running player JavaScript. The pure-Go kkdai/youtube client
// handles that decode for us. High-quality YouTube is delivered as separated
// DASH streams, so Extract returns the best video-only URL and the best
// audio-only URL; muxing the two (e.g. with ffmpeg) is left to the caller.
package youtube

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	yt "github.com/kkdai/youtube/v2"
)

var (
	// ErrNoVideoURL means the input text contains no recognizable YouTube link.
	ErrNoVideoURL = errors.New("youtube: no youtube link found in input")

	// ErrUpstream means fetching or deciphering the video failed.
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

// Service wraps a kkdai youtube client at a fixed quality.
type Service struct {
	client  *yt.Client
	quality Quality
}

// NewService returns a Service at the given quality.
func NewService(q Quality) *Service {
	return &Service{client: &yt.Client{}, quality: q}
}

// Extract finds the first YouTube link in input and returns its stream(s)
// according to the service's quality.
func (s *Service) Extract(input string) (Result, error) {
	link := linkRe.FindString(input)
	if link == "" {
		return Result{}, ErrNoVideoURL
	}

	video, err := s.client.GetVideo(link)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrUpstream, err)
	}

	if s.quality == Muxed {
		return s.extractMuxed(video)
	}
	return s.extractSeparated(video, s.quality == H264)
}

// extractMuxed returns the best single progressive (audio+video) stream.
func (s *Service) extractMuxed(video *yt.Video) (Result, error) {
	f := bestMuxed(video.Formats)
	if f == nil {
		return Result{}, ErrNoStream
	}
	url, err := s.client.GetStreamURL(video, f)
	if err != nil {
		return Result{}, fmt.Errorf("%w: muxed stream: %v", ErrUpstream, err)
	}
	return Result{VideoID: video.ID, Title: video.Title, PlayURL: url}, nil
}

// extractSeparated returns the best video-only and audio-only streams.
func (s *Service) extractSeparated(video *yt.Video, h264 bool) (Result, error) {
	videoFmt := bestVideo(video.Formats, h264)
	audioFmt := bestAudio(video.Formats, h264)
	if videoFmt == nil || audioFmt == nil {
		return Result{}, ErrNoStream
	}

	videoURL, err := s.client.GetStreamURL(video, videoFmt)
	if err != nil {
		return Result{}, fmt.Errorf("%w: video stream: %v", ErrUpstream, err)
	}
	audioURL, err := s.client.GetStreamURL(video, audioFmt)
	if err != nil {
		return Result{}, fmt.Errorf("%w: audio stream: %v", ErrUpstream, err)
	}

	return Result{
		VideoID:  video.ID,
		Title:    video.Title,
		VideoURL: videoURL,
		AudioURL: audioURL,
	}, nil
}

// bestMuxed returns the highest-resolution progressive format (one stream that
// carries both audio and video), or nil if none exist.
func bestMuxed(fl yt.FormatList) *yt.Format {
	var best *yt.Format
	for i := range fl {
		f := &fl[i]
		if f.AudioChannels == 0 || !isMime(f.MimeType, "video/") {
			continue
		}
		if best == nil || f.Height > best.Height ||
			(f.Height == best.Height && f.Bitrate > best.Bitrate) {
			best = f
		}
	}
	return best
}

// bestVideo returns the highest-resolution video-only format (no audio track),
// breaking ties by bitrate. When h264Only is set, only avc1 codecs qualify.
// Returns nil if none match.
func bestVideo(fl yt.FormatList, h264Only bool) *yt.Format {
	var best *yt.Format
	for i := range fl {
		f := &fl[i]
		if f.AudioChannels > 0 || !isMime(f.MimeType, "video/") {
			continue
		}
		if h264Only && !hasCodec(f.MimeType, "avc1") {
			continue
		}
		if best == nil || f.Height > best.Height ||
			(f.Height == best.Height && f.Bitrate > best.Bitrate) {
			best = f
		}
	}
	return best
}

// bestAudio returns the highest-bitrate audio-only format. When aacOnly is set,
// only mp4a (AAC) codecs qualify. Returns nil if none match.
func bestAudio(fl yt.FormatList, aacOnly bool) *yt.Format {
	var best *yt.Format
	for i := range fl {
		f := &fl[i]
		if !isMime(f.MimeType, "audio/") {
			continue
		}
		if aacOnly && !hasCodec(f.MimeType, "mp4a") {
			continue
		}
		if best == nil || f.Bitrate > best.Bitrate {
			best = f
		}
	}
	return best
}

func isMime(mimeType, prefix string) bool {
	return len(mimeType) >= len(prefix) && mimeType[:len(prefix)] == prefix
}

// hasCodec reports whether a YouTube MimeType (e.g. `video/mp4; codecs="avc1.64001f"`)
// declares the given codec.
func hasCodec(mimeType, codec string) bool {
	return strings.Contains(mimeType, codec)
}
