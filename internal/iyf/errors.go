package iyf

import "errors"

var (
	// ErrNoVideoURL means the input text contains no recognizable iyf.tv play
	// link.
	ErrNoVideoURL = errors.New("iyf: no iyf.tv play link found in input")

	// ErrUpstream means an HTTP request to iyf.tv failed, returned a non-2xx
	// status, or the getPlayData API returned a non-200 business code.
	ErrUpstream = errors.New("iyf: upstream request failed")

	// ErrNoStream means the getPlayData response contained no playable stream
	// (e.g. every clarity is VIP-locked and carries no URL).
	ErrNoStream = errors.New("iyf: no playable stream found")
)
