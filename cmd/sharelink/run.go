package main

import (
	"fmt"
	"io"
)

const usage = `sharelink: extract watermark-free Douyin video URLs

Usage:
  sharelink <command> [flags]

Commands:
  parse     Extract a direct link from share text or short URL
  serve     Run as HTTP API service
  version   Print version information

Run 'sharelink <command> -h' for command-specific flags.
`

func printUsage(w io.Writer) {
	fmt.Fprint(w, usage)
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		printUsage(stderr)
		return 2
	}
	switch args[1] {
	case "parse":
		// Real implementation lands in Task 3.
		fmt.Fprintln(stderr, "sharelink: parse not yet implemented")
		return 1
	case "serve":
		// Real implementation lands in Task 4.
		fmt.Fprintln(stderr, "sharelink: serve not yet implemented")
		return 1
	case "version":
		return runVersion(stdout)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "sharelink: unknown command %q\n", args[1])
		printUsage(stderr)
		return 2
	}
}
