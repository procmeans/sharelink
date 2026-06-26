package bilibili

import "errors"

var (
	// ErrNoVideoURL means the input text contains no recognizable Bilibili link.
	ErrNoVideoURL = errors.New("bilibili: no bilibili link found in input")

	// ErrUpstream means an HTTP request to Bilibili failed or returned a non-2xx
	// status / non-zero API code.
	ErrUpstream = errors.New("bilibili: upstream request failed")

	// ErrNoStream means the API response did not contain an extractable play URL.
	ErrNoStream = errors.New("bilibili: no playable stream found")
)
