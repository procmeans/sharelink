package main

import (
	"fmt"
	"io"
)

const usage = `sharelink: extract direct video URLs from Douyin and YouTube

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
		return runParse(args[2:], stdin, stdout, stderr, realExtractorFactory)
	case "serve":
		return runServe(args[2:], stderr)
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
