package main

import (
	"fmt"
	"io"
)

// Set at build time via -ldflags="-X main.version=... -X main.commit=... -X main.date=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func runVersion(stdout io.Writer) int {
	fmt.Fprintf(stdout, "sharelink %s (commit %s, built %s)\n", version, commit, date)
	return 0
}
