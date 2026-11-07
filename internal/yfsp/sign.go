package yfsp

import (
	"crypto/md5"
	"encoding/hex"
	"net/url"
	"strings"
)

// sign reproduces the site's request signature (uriSignature in the web app).
// The signed value is:
//
//	vv = md5( publicKey + "&" + lower(canonicalQuery) + "&" + privateKey )
//
// and the returned URL is base + "?" + query + "&vv=" + vv + "&pub=" + publicKey.
//
// canonicalQuery is query with each value URL-decoded and "+" turned into a
// space (the get_query step), matching the JS exactly.
func sign(base, query, publicKey, privateKey string) string {
	o := strings.ToLower(canonicalQuery(query))
	sum := md5.Sum([]byte(publicKey + "&" + o + "&" + privateKey))
	vv := hex.EncodeToString(sum[:])
	return base + "?" + query + "&vv=" + vv + "&pub=" + publicKey
}

// canonicalQuery mirrors the app's get_query: split on "&", and for each pair
// keep the key verbatim and append each value part URL-decoded with "+"
// rendered as a space. Pairs are rejoined with "&".
func canonicalQuery(query string) string {
	if query == "" {
		return ""
	}
	pairs := strings.Split(query, "&")
	var b strings.Builder
	for i, pair := range pairs {
		if i > 0 {
			b.WriteByte('&')
		}
		parts := strings.Split(pair, "=")
		b.WriteString(parts[0])
		for _, v := range parts[1:] {
			b.WriteByte('=')
			dec, err := url.PathUnescape(v)
			if err != nil {
				dec = v
			}
			b.WriteString(strings.ReplaceAll(dec, "+", " "))
		}
	}
	return b.String()
}
