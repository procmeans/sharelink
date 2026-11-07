package xhs

import "errors"

// Error set mirrors the other providers (bilibili, yfsp) so the HTTP handler
// can group them into the shared error codes.
var (
	// ErrNoVideoURL means the input text contains no Xiaohongshu (xhslink.com
	// or xiaohongshu.com) link.
	ErrNoVideoURL = errors.New("xhs: no xiaohongshu link found in input")

	// ErrUpstream means an HTTP request to Xiaohongshu failed or returned a
	// non-2xx status.
	ErrUpstream = errors.New("xhs: upstream request failed")

	// ErrNoStream means the note page was fetched but no watermark-free media
	// (video stream or image list) could be extracted. This commonly happens
	// when the note URL lacks a valid xsec_token, so Xiaohongshu serves the
	// SPA shell without the inlined note data.
	ErrNoStream = errors.New("xhs: no media found in note page")
)
