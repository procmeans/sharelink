package yfsp

import "errors"

// ErrNoVideoURL means the input text contains no recognizable yfsp.tv play link
// (or the link is missing its id query parameter). Extraction is pure URL
// construction, so this is the only error the service returns.
var ErrNoVideoURL = errors.New("yfsp: no yfsp.tv play link found in input")
