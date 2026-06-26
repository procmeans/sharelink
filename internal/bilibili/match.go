package bilibili

import "regexp"

// linkRe matches the common Bilibili link shapes: a full video page
// (bilibili.com/video/BV... or /av...) or a b23.tv short link. BV ids are
// always "BV" followed by 10 alphanumeric characters.
var linkRe = regexp.MustCompile(
	`https?://(?:(?:www\.|m\.)?bilibili\.com/video/(?:BV[0-9A-Za-z]{10}|av\d+)|b23\.tv/[0-9A-Za-z]+)`)

// bvidRe and aidRe pull the explicit id out of a resolved video-page URL.
var (
	bvidRe = regexp.MustCompile(`BV[0-9A-Za-z]{10}`)
	aidRe  = regexp.MustCompile(`/video/av(\d+)`)
)

// Matches reports whether input contains a recognizable Bilibili link.
func Matches(input string) bool { return linkRe.MatchString(input) }

// refFromURL returns the view/playurl query fragment ("bvid=..." or "aid=...")
// for a resolved Bilibili video-page URL, and whether an id was found.
func refFromURL(link string) (string, bool) {
	if m := bvidRe.FindString(link); m != "" {
		return "bvid=" + m, true
	}
	if m := aidRe.FindStringSubmatch(link); m != nil {
		return "aid=" + m[1], true
	}
	return "", false
}
