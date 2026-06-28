// Package yfsp extracts a direct, playable HLS URL from yfsp.tv (爱壹帆) play
// links.
//
// yfsp.tv is a streaming aggregator that serves video over HLS. Unlike its
// encrypted content/episode API (which is signed and Cloudflare-protected), the
// MasterPlayList endpoint is unauthenticated: given the play URL's id (mediaKey)
// query parameter, https://upload.yfsp.tv/api/video/MasterPlayList?id=<id>
// returns a standard #EXTM3U master playlist whose variants are signed CDN
// chunklist URLs. The master URL itself carries no signature and re-resolves
// fresh, time-limited segment tokens on every fetch, so it is the most robust
// "direct link" to hand back — playable by ffmpeg, yt-dlp or any HLS player.
//
// Extraction is pure URL construction: no network call. The MasterPlayList
// endpoint returns a playlist for any id (a bad id only fails later, when its
// CDN chunklist 404s), so a HEAD/GET here would add latency and false
// confidence without telling us whether the id is real.
package yfsp

const masterPlayListAPI = "https://upload.yfsp.tv/api/video/MasterPlayList?id="

// Result is the yfsp-specific extraction result. The router maps it into the
// platform-neutral extract.Result. yfsp serves HLS, so PlayURL holds the master
// m3u8 URL.
type Result struct {
	ID      string
	PlayURL string
}

// Service resolves yfsp.tv play links to their HLS master playlist URL.
type Service struct{}

// NewService returns a Service.
func NewService() *Service { return &Service{} }

// Extract finds the first yfsp.tv play link in input and returns its HLS master
// playlist URL.
func (s *Service) Extract(input string) (Result, error) {
	link := linkRe.FindString(input)
	if link == "" {
		return Result{}, ErrNoVideoURL
	}
	id, ok := idFromURL(link)
	if !ok {
		return Result{}, ErrNoVideoURL
	}
	return Result{ID: id, PlayURL: masterPlayListAPI + id}, nil
}
