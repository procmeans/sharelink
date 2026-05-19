package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_NoCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"sharelink"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("want exit 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("expected usage on stderr, got: %s", stderr.String())
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"sharelink", "wat"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("want exit 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("expected 'unknown command' on stderr, got: %s", stderr.String())
	}
}

func TestRun_HelpFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"sharelink", "-h"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("want exit 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("expected usage on stdout, got: %s", stdout.String())
	}
}

func TestRun_Version(t *testing.T) {
	// Inject a known version for the duration of this test.
	orig := version
	version = "test-1.2.3"
	defer func() { version = orig }()

	var stdout, stderr bytes.Buffer
	code := run([]string{"sharelink", "version"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("want exit 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "test-1.2.3") {
		t.Fatalf("expected version 'test-1.2.3' on stdout, got: %s", stdout.String())
	}
}
