package tiktok

import "errors"

var (
	// ErrNoURL means the input text contains no TikTok video or short link.
	ErrNoURL = errors.New("tiktok: no video url found in input")
	// ErrUpstream means an HTTP request to TikTok failed, returned a non-2xx
	// status, or did not issue the tt_chain_token cookie.
	ErrUpstream = errors.New("tiktok: upstream request failed")
	// ErrNoPlayURL means the video page did not contain an extractable play URL
	// (captcha page, private/deleted video, photo post, ...).
	ErrNoPlayURL = errors.New("tiktok: play url not found in video page")
)
