package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/procmeans/sharelink/internal/extract"
)

func stubFactory(playURL, videoID string, returnErr error) extractorFactory {
	return stubFactoryResult(extract.Result{PlayURL: playURL, VideoID: videoID}, returnErr)
}

func stubFactoryResult(res extract.Result, returnErr error) extractorFactory {
	return func(withTTWid bool) (func(string) (extract.Result, error), error) {
		return func(input string) (extract.Result, error) {
			if returnErr != nil {
				return extract.Result{}, returnErr
			}
			return res, nil
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

func TestRunParse_JSONOutputIncludesHeaders(t *testing.T) {
	var stdout, stderr bytes.Buffer
	mk := stubFactoryResult(extract.Result{
		PlayURL: "https://v16.tiktok.com/v",
		VideoID: "1",
		Headers: map[string]string{"Referer": "https://www.tiktok.com/"},
	}, nil)
	code := runParse([]string{"--json", "https://www.tiktok.com/@u/video/1"}, strings.NewReader(""), &stdout, &stderr, mk)
	if code != 0 {
		t.Fatalf("want exit 0, got %d (stderr=%s)", code, stderr.String())
	}
	var got struct {
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if got.Headers["Referer"] != "https://www.tiktok.com/" {
		t.Fatalf("want headers in JSON, got %q", stdout.String())
	}
}

func TestRunParse_OutputFileSendsHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "tt_chain_token=t" || r.Header.Get("Referer") != "https://www.tiktok.com/" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte("mp4-bytes"))
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "v.mp4")
	mk := stubFactoryResult(extract.Result{
		PlayURL: srv.URL + "/v",
		Headers: map[string]string{"Cookie": "tt_chain_token=t", "Referer": "https://www.tiktok.com/"},
	}, nil)
	var stdout, stderr bytes.Buffer
	code := runParse([]string{"-o", out, "https://www.tiktok.com/@u/video/1"}, strings.NewReader(""), &stdout, &stderr, mk)
	if code != 0 {
		t.Fatalf("want exit 0, got %d (stderr=%s)", code, stderr.String())
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "mp4-bytes" {
		t.Fatalf("unexpected file content: %q", b)
	}
}

func TestRunParse_OutputFileDownloadFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "v.mp4")
	mk := stubFactory(srv.URL+"/v", "1", nil)
	var stdout, stderr bytes.Buffer
	code := runParse([]string{"-o", out, "https://v.douyin.com/x/"}, strings.NewReader(""), &stdout, &stderr, mk)
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("expected no file left behind, stat err=%v", err)
	}
	if !strings.Contains(stderr.String(), "403") {
		t.Fatalf("expected status on stderr, got: %s", stderr.String())
	}
}
