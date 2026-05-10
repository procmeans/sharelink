package douyin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/procmeans/sharelink/internal/httpclient"
)

const shareBaseURL = "https://www.iesdouyin.com"

// routerDataPrefix is the literal byte sequence preceding the JSON object
// that the iesdouyin share page inlines for SSR'd video metadata.
var routerDataPrefix = []byte("window._ROUTER_DATA = ")

// routerData captures the minimal subset of fields we need from the
// share-page _ROUTER_DATA payload. Extra fields are ignored.
type routerData struct {
	LoaderData struct {
		Page struct {
			VideoInfoRes struct {
				ItemList []struct {
					Video struct {
						PlayAddr struct {
							URLList []string `json:"url_list"`
						} `json:"play_addr"`
					} `json:"video"`
				} `json:"item_list"`
			} `json:"videoInfoRes"`
		} `json:"video_(id)/page"`
	} `json:"loaderData"`
}

func fetchPlayURLFromBase(client *http.Client, cookie, baseURL, videoID string) (string, error) {
	target := fmt.Sprintf("%s/share/video/%s/", baseURL, videoID)

	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", httpclient.MobileUA)
	if cookie != "" {
		req.Header.Set("Cookie", "ttwid="+cookie)
	}

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

	idx := bytes.Index(body, routerDataPrefix)
	if idx < 0 {
		return "", fmt.Errorf("%w: _ROUTER_DATA marker not found", ErrNoPlayURL)
	}
	jsonStart := idx + len(routerDataPrefix)

	var rd routerData
	dec := json.NewDecoder(bytes.NewReader(body[jsonStart:]))
	if err := dec.Decode(&rd); err != nil {
		return "", fmt.Errorf("%w: decode _ROUTER_DATA: %v", ErrNoPlayURL, err)
	}

	items := rd.LoaderData.Page.VideoInfoRes.ItemList
	if len(items) == 0 {
		return "", fmt.Errorf("%w: empty item_list", ErrNoPlayURL)
	}
	urls := items[0].Video.PlayAddr.URLList
	if len(urls) == 0 || urls[0] == "" {
		return "", fmt.Errorf("%w: empty url_list", ErrNoPlayURL)
	}
	return urls[0], nil
}
