package yfsp

import "errors"

var (
	// ErrNoVideoURL means the input text contains no recognizable yfsp.tv play
	// link (or the link is missing its id query parameter).
	ErrNoVideoURL = errors.New("yfsp: no yfsp.tv play link found in input")

	// ErrUpstream means an HTTP request to yfsp.tv failed, returned a non-2xx
	// status, or the play API returned a non-zero business code (e.g. a
	// signature error).
	ErrUpstream = errors.New("yfsp: upstream request failed")

	// ErrNoStream means the play API response contained no playable stream
	// (e.g. every clarity is VIP-locked and disabled for this account).
	ErrNoStream = errors.New("yfsp: no playable stream found")
)
