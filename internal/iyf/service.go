// Package iyf extracts a direct, playable HLS URL from iyf.tv (爱壹帆) play
// links, including the mview.iyf.tv share/preview subdomain.
//
// iyf.tv is the same brand as the sibling yfsp.tv, but its share links carry
// only a content key in the path (/play/<contentKey>) with no ?id= media key,
// and its player resolves the stream through a different, unsigned endpoint.
//
// Flow:
//
//  1. From the play URL take the content key (/play/<contentKey>).
//  2. Call https://api.iyf.tv/api/Video/getPlayData?mediaKey=<contentKey>.
//     The response lists the content's clarities; each is an "episode" with a
//     resolution, a VIP flag, and — for the playable (non-VIP) ones — an inline
//     HLS chunklist mediaUrl. VIP clarities carry an empty mediaUrl.
//  3. Return the highest-resolution playable (non-VIP) stream.
//
// This endpoint needs no request signature (unlike yfsp.tv's video/play API):
// the mediaUrl it returns for the free clarity is the same chunklist the site
// itself plays for anonymous viewers. HD (1080p/720p/4K) clarities are
// typically VIP-locked and carry no URL, so without a logged-in VIP session
// only the standard-definition stream is returned.
package iyf

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/procmeans/sharelink/internal/httpclient"
)

const (
	referer     = "https://www.iyf.tv/"
	playDataAPI = "https://api.iyf.tv/api/Video/getPlayData"
	cinema      = "1"
)

// Result is the iyf-specific extraction result. iyf serves HLS, so PlayURL
// holds the chunklist m3u8 URL. Title is the content title (e.g. the film name)
// and Quality the chosen clarity label (e.g. "480P").
type Result struct {
	ID      string
	Title   string
	Quality string
	PlayURL string
}

// Service resolves iyf.tv play links to a playable HLS URL via the unsigned
// getPlayData API.
type Service struct {
	client *http.Client
}

// NewService returns a Service backed by a redirect-following HTTP client.
func NewService() *Service {
	return &Service{client: httpclient.NewFollowing()}
}

// Extract finds the first iyf.tv play link in input and returns the playable
// HLS stream for that content.
func (s *Service) Extract(input string) (Result, error) {
	key, ok := keyFromURL(input)
	if !ok {
		return Result{}, ErrNoVideoURL
	}

	episodes, err := s.fetchPlayData(key)
	if err != nil {
		return Result{}, err
	}

	ep := bestStream(episodes)
	if ep == nil {
		return Result{}, ErrNoStream
	}
	return Result{ID: key, Title: ep.Title, Quality: ep.ResolutionDes, PlayURL: ep.MediaURL}, nil
}

// episode is one clarity entry in the getPlayData list.
type episode struct {
	Title         string `json:"title"`
	IsVip         bool   `json:"isVip"`
	Resolution    string `json:"resolution"`
	ResolutionDes string `json:"resolutionDes"`
	MediaURL      string `json:"mediaUrl"`
}

type playDataResponse struct {
	Ret  int    `json:"ret"`
	Msg  string `json:"msg"`
	Data struct {
		List []episode `json:"list"`
	} `json:"data"`
}

// fetchPlayData calls the getPlayData API for the given content key and returns
// the clarity list.
func (s *Service) fetchPlayData(key string) ([]episode, error) {
	url := playDataAPI + "?mediaKey=" + key + "&cinema=" + cinema

	body, err := s.get(url)
	if err != nil {
		return nil, err
	}
	var pr playDataResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("%w: decode getPlayData response: %v", ErrUpstream, err)
	}
	if pr.Ret != 200 {
		return nil, fmt.Errorf("%w: getPlayData ret %d: %s", ErrUpstream, pr.Ret, pr.Msg)
	}
	if len(pr.Data.List) == 0 {
		return nil, ErrNoStream
	}
	return pr.Data.List, nil
}

// bestStream returns the highest-resolution playable clarity among the
// episodes (a non-VIP entry that carries a stream URL), or nil if none.
func bestStream(episodes []episode) *episode {
	best := -1
	var chosen *episode
	for i := range episodes {
		ep := &episodes[i]
		if ep.IsVip || ep.MediaURL == "" {
			continue
		}
		res, _ := strconv.Atoi(ep.Resolution)
		if res > best {
			best = res
			chosen = ep
		}
	}
	return chosen
}

// get performs a GET with the iyf headers and returns the response body.
func (s *Service) get(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", httpclient.MobileUA)
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", "application/json")

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
