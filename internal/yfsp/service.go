// Package yfsp extracts a direct, playable HLS URL from yfsp.tv (爱壹帆) play
// links via the site's official video/play API.
//
// Flow:
//
//  1. From the play URL (/play/<contentKey>?id=<mediaKey>) take the mediaKey.
//  2. Fetch the play page HTML. Each response inlines a fresh, single-use
//     signing key pair in window.injectJson's pConfig, plus the viewer's
//     region. We scrape both.
//  3. Sign the video/play request (md5 over publicKey&query&privateKey, see
//     sign.go) and call https://m10.yfsp.tv/v3/video/play.
//  4. The response lists the episode's clarities. Each playable one carries an
//     HLS chunklist URL; VIP-only clarities are disabled with no URL. We return
//     the highest-bitrate playable (non-VIP) stream.
//
// The earlier upload.{Host}/MasterPlayList endpoint is intentionally not used:
// it is unauthenticated but returns inconsistent/placeholder content for these
// media keys. The signed video/play API is the authoritative source.
//
// HD (1080p/720p) clarities are typically VIP-locked; without a logged-in VIP
// session only the standard-definition stream is returned, mirroring how the
// site itself plays for anonymous viewers.
package yfsp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/procmeans/sharelink/internal/httpclient"
)

const (
	referer    = "https://www.yfsp.tv/"
	playAPI    = "https://m10.yfsp.tv/v3/video/play"
	cinema     = "1"
	defaultReg = "GL."
)

// pConfigRe pulls the inlined signing key pair (publicKey, privateKey[0]) out of
// the play page HTML. privateKey is an array in the page; we take its first
// element (the app uses index publicKeyInedx%len, which is 0).
var pConfigRe = regexp.MustCompile(`"pConfig":\{"publicKey":"([^"]+)","privateKey":\[?"([^"]+)"`)

// regionRe pulls the viewer's region code (used in the play query/signature).
var regionRe = regexp.MustCompile(`"switch-region":\[\{[^}]*"regionCode":"([^"]*)"`)

// Result is the yfsp-specific extraction result. yfsp serves HLS, so PlayURL
// holds the chunklist m3u8 URL. Title is the episode label (e.g. "44") and
// Quality the chosen clarity (e.g. "576").
type Result struct {
	ID      string
	Title   string
	Quality string
	PlayURL string
}

// Service resolves yfsp.tv play links to a playable HLS URL via the signed
// video/play API.
type Service struct {
	client *http.Client
}

// NewService returns a Service backed by a redirect-following HTTP client.
func NewService() *Service {
	return &Service{client: httpclient.NewFollowing()}
}

// Extract finds the first yfsp.tv play link in input and returns the playable
// HLS stream for that episode.
func (s *Service) Extract(input string) (Result, error) {
	link := linkRe.FindString(input)
	if link == "" {
		return Result{}, ErrNoVideoURL
	}
	id, ok := idFromURL(link)
	if !ok {
		return Result{}, ErrNoVideoURL
	}

	pub, priv, region, err := s.fetchSigningContext(link)
	if err != nil {
		return Result{}, err
	}

	info, err := s.fetchPlay(id, region, pub, priv)
	if err != nil {
		return Result{}, err
	}

	playURL, quality := bestStream(info.Clarity)
	if playURL == "" {
		return Result{}, ErrNoStream
	}
	return Result{ID: id, Title: info.MediaTitle, Quality: quality, PlayURL: playURL}, nil
}

// fetchSigningContext loads the play page and scrapes the fresh, single-use
// signing key pair and region code that the page's own JS would use.
func (s *Service) fetchSigningContext(pageURL string) (pub, priv, region string, err error) {
	body, err := s.get(pageURL, "text/html")
	if err != nil {
		return "", "", "", err
	}
	m := pConfigRe.FindSubmatch(body)
	if m == nil {
		return "", "", "", fmt.Errorf("%w: pConfig signing keys not found in play page", ErrUpstream)
	}
	region = defaultReg
	if r := regionRe.FindSubmatch(body); r != nil && len(r[1]) > 0 {
		region = string(r[1])
	}
	return string(m[1]), string(m[2]), region, nil
}

// playInfo is the subset of the video/play response we consume.
type playInfo struct {
	MediaTitle string         `json:"mediaTitle"`
	MediaKey   string         `json:"mediaKey"`
	Clarity    []clarityEntry `json:"clarity"`
}

type clarityEntry struct {
	Title     string `json:"title"`
	Bitrate   int    `json:"bitrate"`
	IsVIP     bool   `json:"isVIP"`
	IsEnabled bool   `json:"isEnabled"`
	Path      *struct {
		IsHls  bool   `json:"isHls"`
		Result string `json:"result"`
	} `json:"path"`
}

type playResponse struct {
	Ret  int `json:"ret"`
	Data struct {
		Code int        `json:"code"`
		Msg  string     `json:"msg"`
		Info []playInfo `json:"info"`
	} `json:"data"`
}

// fetchPlay signs and calls the video/play API for the given media key.
func (s *Service) fetchPlay(id, region, pub, priv string) (playInfo, error) {
	query := "cinema=" + cinema + "&id=" + id +
		"&a=0&lang=none&usersign=1&region=" + region + "&device=1&isMasterSupport=1"
	url := sign(playAPI, query, pub, priv)

	body, err := s.get(url, "application/json")
	if err != nil {
		return playInfo{}, err
	}
	var pr playResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return playInfo{}, fmt.Errorf("%w: decode play response: %v", ErrUpstream, err)
	}
	if pr.Data.Code != 0 {
		return playInfo{}, fmt.Errorf("%w: play api code %d: %s", ErrUpstream, pr.Data.Code, pr.Data.Msg)
	}
	if len(pr.Data.Info) == 0 {
		return playInfo{}, ErrNoStream
	}
	return pr.Data.Info[0], nil
}

// bestStream returns the highest-bitrate playable HLS URL among the clarities
// (an enabled clarity that carries a stream URL), and its quality label. VIP
// clarities are disabled with no URL and are skipped.
func bestStream(clarities []clarityEntry) (url, quality string) {
	best := -1
	for _, c := range clarities {
		if !c.IsEnabled || c.Path == nil || c.Path.Result == "" {
			continue
		}
		if c.Bitrate > best {
			best = c.Bitrate
			url = c.Path.Result
			quality = c.Title
		}
	}
	return url, quality
}

// get performs a GET with the yfsp headers and returns the response body.
func (s *Service) get(url, accept string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", httpclient.MobileUA)
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", accept)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: do request: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: unexpected status %d", ErrUpstream, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrUpstream, err)
	}
	return body, nil
}
