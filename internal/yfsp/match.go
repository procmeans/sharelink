package yfsp

import "regexp"

// linkRe matches a yfsp.tv (爱壹帆) play-page URL: /play/<contentKey> followed
// by a query string. The mediaKey we resolve is the id query parameter, pulled
// out by idFromURL.
var linkRe = regexp.MustCompile(
	`https?://(?:www\.|m\.)?yfsp\.tv/play/[0-9A-Za-z]+\?\S+`)

// idRe pulls the id (mediaKey) query parameter out of a play URL.
var idRe = regexp.MustCompile(`[?&]id=([0-9A-Za-z]+)`)

// Matches reports whether input contains a yfsp.tv play link that carries an
// id parameter (without it there is nothing to resolve).
func Matches(input string) bool {
	link := linkRe.FindString(input)
	if link == "" {
		return false
	}
	_, ok := idFromURL(link)
	return ok
}

// idFromURL returns the id (mediaKey) query parameter of a play URL, and
// whether one was present.
func idFromURL(link string) (string, bool) {
	m := idRe.FindStringSubmatch(link)
	if m == nil {
		return "", false
	}
	return m[1], true
}
