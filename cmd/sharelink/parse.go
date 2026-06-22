package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/procmeans/sharelink/internal/cache"
	"github.com/procmeans/sharelink/internal/douyin"
	"github.com/procmeans/sharelink/internal/extract"
	"github.com/procmeans/sharelink/internal/httpclient"
	"github.com/procmeans/sharelink/internal/youtube"
)

// extractorOpts carries the parse flags that influence how the extractor is
// built.
type extractorOpts struct {
	withTTWid bool
	quality   youtube.Quality
}

// extractorFactory returns an extract function for the given options. The real
// factory builds a Router; tests inject stubs.
type extractorFactory func(opts extractorOpts) (func(input string) (extract.Result, error), error)

func runParse(args []string, stdin io.Reader, stdout, stderr io.Writer, mkExtractor extractorFactory) int {
	fs := flag.NewFlagSet("parse", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "output JSON instead of plain URL")
	withTTWid := fs.Bool("with-ttwid", false, "fetch ttwid cookie before parsing (Douyin only)")
	hq := fs.Bool("hq", false, "YouTube: highest-quality separated streams, any codec (VP9/AV1, up to 4K)")
	mp4 := fs.Bool("mp4", false, "YouTube: highest-quality H.264/AAC separated streams (up to 1080p)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	input, err := readInput(fs.Args(), stdin)
	if err != nil {
		fmt.Fprintln(stderr, "sharelink:", err)
		return 2
	}

	extractFn, err := mkExtractor(extractorOpts{withTTWid: *withTTWid, quality: youtubeQuality(*hq, *mp4)})
	if err != nil {
		fmt.Fprintln(stderr, "sharelink:", err)
		return 1
	}

	res, err := extractFn(input)
	if err != nil {
		fmt.Fprintln(stderr, "sharelink:", err)
		return 1
	}

	if *jsonOut {
		return writeJSON(stdout, stderr, res)
	}
	for _, line := range plainURLs(res) {
		fmt.Fprintln(stdout, line)
	}
	return 0
}

// youtubeQuality maps the --hq/--mp4 flags to a quality. The default (neither
// flag) is a single muxed low-quality stream that is audio+video synced and
// directly playable. --mp4 takes precedence over --hq, since it is the more
// specific (codec-constrained) request.
func youtubeQuality(hq, mp4 bool) youtube.Quality {
	switch {
	case mp4:
		return youtube.H264
	case hq:
		return youtube.Best
	default:
		return youtube.Muxed
	}
}

// plainURLs returns the URL line(s) for plain-text output. Douyin yields a
// single play URL; YouTube yields the video stream followed by the audio stream.
func plainURLs(res extract.Result) []string {
	if res.PlayURL != "" {
		return []string{res.PlayURL}
	}
	var out []string
	if res.VideoURL != "" {
		out = append(out, res.VideoURL)
	}
	if res.AudioURL != "" {
		out = append(out, res.AudioURL)
	}
	return out
}

// readInput returns input from extraArgs[0] if any, else reads all of stdin.
func readInput(extraArgs []string, stdin io.Reader) (string, error) {
	if len(extraArgs) > 0 {
		return strings.TrimSpace(extraArgs[0]), nil
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", errors.New("no input on argv or stdin")
	}
	return s, nil
}

func writeJSON(stdout, stderr io.Writer, res extract.Result) int {
	out := struct {
		Source   string `json:"source,omitempty"`
		PlayURL  string `json:"play_url,omitempty"`
		VideoURL string `json:"video_url,omitempty"`
		AudioURL string `json:"audio_url,omitempty"`
		VideoID  string `json:"video_id,omitempty"`
		Title    string `json:"title,omitempty"`
	}{
		Source:   res.Source,
		PlayURL:  res.PlayURL,
		VideoURL: res.VideoURL,
		AudioURL: res.AudioURL,
		VideoID:  res.VideoID,
		Title:    res.Title,
	}
	b, err := json.Marshal(out)
	if err != nil {
		fmt.Fprintln(stderr, "sharelink:", err)
		return 1
	}
	fmt.Fprintln(stdout, string(b))
	return 0
}

// realExtractorFactory constructs a Router over the Douyin and YouTube
// providers with a no-op cache and optional ttwid manager, then returns the
// bound Extract method.
func realExtractorFactory(opts extractorOpts) (func(string) (extract.Result, error), error) {
	c := cache.New(0, 0)
	var ttwid *douyin.TTWidManager
	if opts.withTTWid {
		ttwid = douyin.NewTTWidManager(httpclient.NewNoRedirect())
		if err := ttwid.Refresh(); err != nil {
			return nil, fmt.Errorf("ttwid refresh: %w", err)
		}
	}
	router := extract.NewRouter(douyin.NewService(ttwid, c), youtube.NewService(opts.quality))
	return router.Extract, nil
}
