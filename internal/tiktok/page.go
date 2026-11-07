package tiktok

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"

	"github.com/procmeans/sharelink/internal/httpclient"
)

const referer = "https://www.tiktok.com/"

// desktopUA is used instead of httpclient.MobileUA: mobile UAs get the
// "reflow" page, whose data lives under a different key
// (webapp.reflow.video.detail) than the one parsed here.
const desktopUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

// dataPrefix is the opening tag of the SSR JSON blob on a TikTok video page.
var dataPrefix = []byte(`<script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">`)

// universalData captures the minimal subset of fields we need. Extra fields
// are ignored.
type universalData struct {
	DefaultScope struct {
		VideoDetail struct {
			StatusCode int    `json:"statusCode"`
			StatusMsg  string `json:"statusMsg"`
			ItemInfo   struct {
				ItemStruct struct {
					ID    string `json:"id"`
					Video struct {
						// playAddr is watermark-free; downloadAddr carries the TikTok logo.
						PlayAddr string `json:"playAddr"`
					} `json:"video"`
				} `json:"itemStruct"`
			} `json:"itemInfo"`
		} `json:"webapp.video-detail"`
	} `json:"__DEFAULT_SCOPE__"`
}

// Result is what Extract returns.
type Result struct {
	PlayURL string
	VideoID string
	// Headers must be sent when fetching PlayURL, or the CDN answers 403.
	Headers map[string]string
}

// Service extracts TikTok play URLs. It is stateless: each Extract uses a
// fresh cookie jar so the returned tt_chain_token matches the returned URL.
type Service struct{}

// NewService returns a TikTok Service.
func NewService() *Service { return &Service{} }

// Extract takes user-pasted text or a TikTok link and returns the
// watermark-free play URL plus the headers needed to download it.
func (s *Service) Extract(input string) (Result, error) {
	pageURL, err := ExtractURL(input)
	if err != nil {
		return Result{}, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return Result{}, err
	}
	client := httpclient.NewFollowing()
	client.Jar = jar
	return fetch(client, pageURL)
}

// fetch GETs pageURL (following short-link redirects), parses the play URL
// out of the page, and reads tt_chain_token from client's cookie jar.
func fetch(client *http.Client, pageURL string) (Result, error) {
	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", desktopUA)

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("%w: do request: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("%w: unexpected status %d", ErrUpstream, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, fmt.Errorf("%w: read body: %v", ErrUpstream, err)
	}

	idx := bytes.Index(body, dataPrefix)
	if idx < 0 {
		return Result{}, fmt.Errorf("%w: __UNIVERSAL_DATA_FOR_REHYDRATION__ not found", ErrNoPlayURL)
	}
	var ud universalData
	if err := json.NewDecoder(bytes.NewReader(body[idx+len(dataPrefix):])).Decode(&ud); err != nil {
		return Result{}, fmt.Errorf("%w: decode page data: %v", ErrNoPlayURL, err)
	}
	vd := ud.DefaultScope.VideoDetail
	if vd.StatusCode != 0 {
		return Result{}, fmt.Errorf("%w: status %d %s", ErrNoPlayURL, vd.StatusCode, vd.StatusMsg)
	}
	item := vd.ItemInfo.ItemStruct
	if item.Video.PlayAddr == "" {
		return Result{}, fmt.Errorf("%w: empty playAddr", ErrNoPlayURL)
	}

	var token string
	for _, ck := range client.Jar.Cookies(resp.Request.URL) {
		if ck.Name == "tt_chain_token" {
			token = ck.Value
			break
		}
	}
	if token == "" {
		return Result{}, fmt.Errorf("%w: tt_chain_token cookie not set", ErrUpstream)
	}

	return Result{
		PlayURL: item.Video.PlayAddr,
		VideoID: item.ID,
		Headers: map[string]string{
			"Cookie":  "tt_chain_token=" + token,
			"Referer": referer,
		},
	}, nil
}
