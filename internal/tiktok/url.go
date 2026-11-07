package tiktok

import "regexp"

// urlRe matches a full video URL (www./m.tiktok.com/@user/video/<id>) or a
// share short link (vm./vt.tiktok.com/<code>, www.tiktok.com/t/<code>).
var urlRe = regexp.MustCompile(`https?://(?:(?:www|m)\.tiktok\.com/@[^/\s]+/video/\d+|(?:vm|vt)\.tiktok\.com/[A-Za-z0-9]+/?|www\.tiktok\.com/t/[A-Za-z0-9]+/?)`)

// Matches reports whether input contains a TikTok video or short link.
func Matches(input string) bool { return urlRe.MatchString(input) }

// ExtractURL finds the first TikTok video or short link in input.
// Returns ErrNoURL if none is present.
func ExtractURL(input string) (string, error) {
	match := urlRe.FindString(input)
	if match == "" {
		return "", ErrNoURL
	}
	return match, nil
}
