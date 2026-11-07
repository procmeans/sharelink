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
	"github.com/procmeans/sharelink/internal/tiktok"
)

// extractorFactory returns an extract function given the --with-ttwid flag.
// The real factory builds a Service; tests inject stubs.
type extractorFactory func(withTTWid bool) (func(input string) (extract.Result, error), error)

func runParse(args []string, stdin io.Reader, stdout, stderr io.Writer, mkExtractor extractorFactory) int {
	fs := flag.NewFlagSet("parse", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "output JSON instead of plain URL")
	withTTWid := fs.Bool("with-ttwid", false, "fetch ttwid cookie before parsing")
	outPath := fs.String("o", "", "also download the video to this file")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	input, err := readInput(fs.Args(), stdin)
	if err != nil {
		fmt.Fprintln(stderr, "sharelink:", err)
		return 2
	}

	extract, err := mkExtractor(*withTTWid)
	if err != nil {
		fmt.Fprintln(stderr, "sharelink:", err)
		return 1
	}

	res, err := extract(input)
	if err != nil {
		fmt.Fprintln(stderr, "sharelink:", err)
		return 1
	}
	if *outPath != "" {
		if err := download(res.PlayURL, res.Headers, *outPath); err != nil {
			fmt.Fprintln(stderr, "sharelink:", err)
			return 1
		}
	}

	if *jsonOut {
		return writeJSON(stdout, stderr, res)
	}
	fmt.Fprintln(stdout, res.PlayURL)
	return 0
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
		PlayURL string            `json:"play_url"`
		VideoID string            `json:"video_id"`
		Headers map[string]string `json:"headers,omitempty"`
	}{
		PlayURL: res.PlayURL,
		VideoID: res.VideoID,
		Headers: res.Headers,
	}
	b, err := json.Marshal(out)
	if err != nil {
		fmt.Fprintln(stderr, "sharelink:", err)
		return 1
	}
	fmt.Fprintln(stdout, string(b))
	return 0
}

// realExtractorFactory constructs a Douyin Service with a no-op cache and
// optional ttwid manager, and returns a Router's Extract method.
func realExtractorFactory(withTTWid bool) (func(string) (extract.Result, error), error) {
	c := cache.New(0, 0)
	var ttwid *douyin.TTWidManager
	if withTTWid {
		ttwid = douyin.NewTTWidManager(httpclient.NewNoRedirect())
		if err := ttwid.Refresh(); err != nil {
			return nil, fmt.Errorf("ttwid refresh: %w", err)
		}
	}
	r := extract.Router{Douyin: douyin.NewService(ttwid, c).Extract, TikTok: tiktok.Extract}
	return r.Extract, nil
}
