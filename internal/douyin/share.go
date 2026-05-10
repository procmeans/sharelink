package douyin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"

	"github.com/procmeans/sharelink/internal/httpclient"
)

const shareBaseURL = "https://www.iesdouyin.com"

var renderDataRe = regexp.MustCompile(`<script id="RENDER_DATA"[^>]*>([^<]+)</script>`)

// renderData captures the minimal subset of fields we need from the
// share-page RENDER_DATA payload. Extra fields are ignored.
type renderData struct {
	App struct {
		AwemeDetail struct {
			Video struct {
				PlayAddr struct {
					URLList []string `json:"url_list"`
				} `json:"play_addr"`
			} `json:"video"`
		} `json:"aweme_detail"`
	} `json:"app"`
}

// FetchPlayURL fetches the share page for videoID and returns the first
// (still-watermarked) play URL from RENDER_DATA. Use StripWatermark on the
// result to remove the watermark.
func FetchPlayURL(client *http.Client, videoID string) (string, error) {
	return fetchPlayURLFromBase(client, shareBaseURL, videoID)
}

func fetchPlayURLFromBase(client *http.Client, baseURL, videoID string) (string, error) {
	target := fmt.Sprintf("%s/share/video/%s/", baseURL, videoID)

	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", httpclient.MobileUA)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: do request: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: unexpected status %d", ErrUpstream, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: read body: %v", ErrUpstream, err)
	}

	m := renderDataRe.FindSubmatch(body)
	if len(m) < 2 {
		return "", fmt.Errorf("%w: RENDER_DATA script not found", ErrNoPlayURL)
	}

	decoded, err := url.QueryUnescape(string(m[1]))
	if err != nil {
		return "", fmt.Errorf("%w: decode RENDER_DATA: %v", ErrNoPlayURL, err)
	}

	var rd renderData
	if err := json.Unmarshal([]byte(decoded), &rd); err != nil {
		return "", fmt.Errorf("%w: unmarshal RENDER_DATA: %v", ErrNoPlayURL, err)
	}

	urls := rd.App.AwemeDetail.Video.PlayAddr.URLList
	if len(urls) == 0 || urls[0] == "" {
		return "", fmt.Errorf("%w: empty url_list", ErrNoPlayURL)
	}
	return urls[0], nil
}
