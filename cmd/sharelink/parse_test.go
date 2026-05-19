package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/procmeans/sharelink/internal/douyin"
)

func stubFactory(playURL, videoID string, returnErr error) extractorFactory {
	return func(withTTWid bool) (func(string) (douyin.Result, error), error) {
		return func(input string) (douyin.Result, error) {
			if returnErr != nil {
				return douyin.Result{}, returnErr
			}
			return douyin.Result{PlayURL: playURL, VideoID: videoID}, nil
		}, nil
	}
}

func TestRunParse_ArgvInput_PlainOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	mk := stubFactory("https://cdn/x.mp4", "7000", nil)
	code := runParse([]string{"https://v.douyin.com/xxx/"}, strings.NewReader(""), &stdout, &stderr, mk)
	if code != 0 {
		t.Fatalf("want exit 0, got %d (stderr=%s)", code, stderr.String())
	}
	if stdout.String() != "https://cdn/x.mp4\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", stderr.String())
	}
}

func TestRunParse_StdinInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	mk := stubFactory("https://cdn/x.mp4", "7000", nil)
	code := runParse(nil, strings.NewReader("text with https://v.douyin.com/xxx/ embedded\n"), &stdout, &stderr, mk)
	if code != 0 {
		t.Fatalf("want exit 0, got %d (stderr=%s)", code, stderr.String())
	}
	if stdout.String() != "https://cdn/x.mp4\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRunParse_JSONOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	mk := stubFactory("https://cdn/x.mp4", "7000", nil)
	code := runParse([]string{"--json", "https://v.douyin.com/xxx/"}, strings.NewReader(""), &stdout, &stderr, mk)
	if code != 0 {
		t.Fatalf("want exit 0, got %d (stderr=%s)", code, stderr.String())
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(stdout.String()), &got); err != nil {
		t.Fatalf("stdout not valid JSON: %v (got %q)", err, stdout.String())
	}
	if got["play_url"] != "https://cdn/x.mp4" {
		t.Fatalf("want play_url, got %q", got["play_url"])
	}
	if got["video_id"] != "7000" {
		t.Fatalf("want video_id, got %q", got["video_id"])
	}
}

func TestRunParse_NoInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	mk := stubFactory("", "", nil)
	code := runParse(nil, strings.NewReader(""), &stdout, &stderr, mk)
	if code != 2 {
		t.Fatalf("want exit 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "no input") {
		t.Fatalf("expected 'no input' on stderr, got: %s", stderr.String())
	}
}

func TestRunParse_ExtractError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	mk := stubFactory("", "", errors.New("douyin: upstream request failed: blah"))
	code := runParse([]string{"https://v.douyin.com/xxx/"}, strings.NewReader(""), &stdout, &stderr, mk)
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout on error, got: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "upstream") {
		t.Fatalf("expected error on stderr, got: %s", stderr.String())
	}
}

func TestRunParse_UnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	mk := stubFactory("", "", nil)
	code := runParse([]string{"--wat"}, strings.NewReader(""), &stdout, &stderr, mk)
	if code != 2 {
		t.Fatalf("want exit 2 on bad flag, got %d", code)
	}
}
