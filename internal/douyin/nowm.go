package douyin

import "strings"

// StripWatermark replaces the first "playwm" segment in url with "play",
// converting a watermarked Douyin CDN URL into a watermark-free one.
// If "playwm" is not present, the URL is returned unchanged.
func StripWatermark(url string) string {
	return strings.Replace(url, "playwm", "play", 1)
}
