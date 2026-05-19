// Package main is the sharelink CLI entrypoint.
package main

import "os"

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}
