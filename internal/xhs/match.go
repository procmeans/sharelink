package xhs

import "regexp"

// linkRe matches the two Xiaohongshu link shapes we accept:
//
//   - a xhslink.com share short link (any path: /a/<code>, /o/<code>, ...),
//     which redirects to the canonical note URL carrying the xsec_token; and
//   - a canonical note page on xiaohongshu.com (/explore/<id> or
//     /discovery/item/<id>), optionally followed by a query string.
//
// We deliberately keep the short-link arm broad (\S+ after the host) because
// the redirect, not the URL shape, is what resolves the note.
var linkRe = regexp.MustCompile(
	`https?://(?:www\.)?(?:xhslink\.com/\S+|xiaohongshu\.com/(?:explore|discovery/item)/[0-9a-fA-F]+(?:\?\S*)?)`)

// shortLinkRe reports whether a matched link is a xhslink.com short link that
// must be resolved via an HTTP redirect to obtain the note URL and xsec_token.
var shortLinkRe = regexp.MustCompile(`https?://(?:www\.)?xhslink\.com/`)

// noteIDRe pulls the 24-hex note id out of a canonical xiaohongshu.com note URL.
var noteIDRe = regexp.MustCompile(`/(?:explore|discovery/item)/([0-9a-fA-F]+)`)

// Matches reports whether input contains a recognizable Xiaohongshu link.
func Matches(input string) bool { return linkRe.MatchString(input) }
