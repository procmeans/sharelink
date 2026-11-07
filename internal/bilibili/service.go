// Package bilibili extracts a direct, playable video URL from Bilibili
// watch/share links.
//
// Unlike Douyin (whose share page inlines a ready mp4 URL), Bilibili exposes
// streams through a JSON API: first resolve the video's cid via the
// web-interface/view endpoint, then ask player/playurl for a stream. We request
// the html5 platform's progressive ("durl") MP4, which is a single audio+video
// muxed URL that is directly playable — no muxing needed, mirroring Douyin's
// PlayURL. (Higher resolutions and DASH streams require a logged-in SESSDATA
// cookie and are out of scope.) All API and stream URLs require a
// bilibili.com Referer.
package bilibili

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/procmeans/sharelink/internal/httpclient"
)

const (
	referer    = "https://www.bilibili.com"
	viewAPI    = "https://api.bilibili.com/x/web-interface/view"
	playurlAPI = "https://api.bilibili.com/x/player/playurl"
)

// Result is the Bilibili-specific extraction result. The router maps it into
// the platform-neutral extract.Result. Bilibili returns a single muxed MP4, so
// only PlayURL is set.
type Result struct {
	BVID    string
	Title   string
	PlayURL string
}

// Service fetches Bilibili stream URLs over the public JSON API.
type Service struct {
	client *http.Client
}

// NewService returns a Service backed by a redirect-following HTTP client
// (needed to resolve b23.tv short links).
func NewService() *Service {
	return &Service{client: httpclient.NewFollowing()}
}

// Extract finds the first Bilibili link in input and returns its muxed play URL.
func (s *Service) Extract(input string) (Result, error) {
	ref, err := s.resolveRef(input)
	if err != nil {
		return Result{}, err
	}

	view, err := s.fetchView(ref)
	if err != nil {
		return Result{}, err
	}

	playURL, err := s.fetchPlayURL(view.Data.BVID, view.Data.CID)
	if err != nil {
		return Result{}, err
	}

	return Result{BVID: view.Data.BVID, Title: view.Data.Title, PlayURL: playURL}, nil
}

// resolveRef finds the first Bilibili link in input and returns the API query
// fragment ("bvid=..." or "aid=..."), following a b23.tv short link if needed.
func (s *Service) resolveRef(input string) (string, error) {
	link := linkRe.FindString(input)
	if link == "" {
		return "", ErrNoVideoURL
	}
	if strings.Contains(link, "b23.tv/") {
		resolved, err := s.followShortLink(link)
		if err != nil {
			return "", err
		}
		link = resolved
	}
	if ref, ok := refFromURL(link); ok {
		return ref, nil
	}
	return "", ErrNoVideoURL
}

// followShortLink resolves a b23.tv short link to its final video-page URL.
func (s *Service) followShortLink(short string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, short, nil)
	if err != nil {
		return "", fmt.Errorf("%w: build short-link request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", httpclient.MobileUA)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: resolve short link: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	return resp.Request.URL.String(), nil
}

type viewResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		BVID  string `json:"bvid"`
		Title string `json:"title"`
		CID   int64  `json:"cid"`
	} `json:"data"`
}

type playurlResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Quality int `json:"quality"`
		Durl    []struct {
			URL string `json:"url"`
		} `json:"durl"`
	} `json:"data"`
}

// fetchView resolves the video's cid (and canonical bvid/title) from the
// view endpoint. ref is "bvid=..." or "aid=...".
func (s *Service) fetchView(ref string) (viewResponse, error) {
	var vr viewResponse
	if err := s.getJSON(viewAPI+"?"+ref, &vr); err != nil {
		return viewResponse{}, err
	}
	if vr.Code != 0 {
		return viewResponse{}, fmt.Errorf("%w: view api code %d: %s", ErrUpstream, vr.Code, vr.Message)
	}
	if vr.Data.BVID == "" || vr.Data.CID == 0 {
		return viewResponse{}, fmt.Errorf("%w: view api returned no cid", ErrNoStream)
	}
	return vr, nil
}

// fetchPlayURL requests the html5 progressive MP4 for the given bvid+cid and
// returns its single muxed URL.
func (s *Service) fetchPlayURL(bvid string, cid int64) (string, error) {
	q := fmt.Sprintf("?bvid=%s&cid=%d&qn=80&platform=html5&high_quality=1&fnval=1", bvid, cid)
	var pr playurlResponse
	if err := s.getJSON(playurlAPI+q, &pr); err != nil {
		return "", err
	}
	if pr.Code != 0 {
		return "", fmt.Errorf("%w: playurl api code %d: %s", ErrUpstream, pr.Code, pr.Message)
	}
	if len(pr.Data.Durl) == 0 || pr.Data.Durl[0].URL == "" {
		return "", fmt.Errorf("%w: playurl api returned no durl", ErrNoStream)
	}
	return pr.Data.Durl[0].URL, nil
}

// getJSON performs a GET with the required Bilibili headers and decodes the
// JSON body into v.
func (s *Service) getJSON(url string, v any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", httpclient.MobileUA)
	req.Header.Set("Referer", referer)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: do request: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: unexpected status %d", ErrUpstream, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: read body: %v", ErrUpstream, err)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%w: decode body: %v", ErrUpstream, err)
	}
	return nil
}
