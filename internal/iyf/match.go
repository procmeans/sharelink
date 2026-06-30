package iyf

import "regexp"

// linkRe matches an iyf.tv (爱壹帆) play-page URL: <sub>.iyf.tv/play/<contentKey>.
// mview is the share/preview subdomain; www and m are the desktop/mobile sites.
// The trailing path segment is the content key we resolve via getPlayData (it
// carries no ?id= the way the sibling yfsp.tv links do).
var linkRe = regexp.MustCompile(
	`https?://(?:www\.|m\.|mview\.)?iyf\.tv/play/([0-9A-Za-z]+)`)

// Matches reports whether input contains an iyf.tv play link.
func Matches(input string) bool {
	return linkRe.MatchString(input)
}

// keyFromURL returns the content key of the first iyf.tv play link in input,
// and whether one was present.
func keyFromURL(input string) (string, bool) {
	m := linkRe.FindStringSubmatch(input)
	if m == nil {
		return "", false
	}
	return m[1], true
}
