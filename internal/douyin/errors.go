package douyin

import "errors"

var (
	// ErrNoShortURL means the input text contains no v.douyin.com link.
	ErrNoShortURL = errors.New("douyin: no short url found in input")

	// ErrInvalidRedirect means the 302 target did not contain a parseable video id.
	ErrInvalidRedirect = errors.New("douyin: redirect target has no video id")

	// ErrUpstream means an HTTP request to Douyin failed or returned a non-2xx status.
	ErrUpstream = errors.New("douyin: upstream request failed")

	// ErrNoPlayURL means the share page did not contain an extractable play URL.
	ErrNoPlayURL = errors.New("douyin: play url not found in share page")
)
