# sharelink CLI + Homebrew Distribution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure sharelink as a single-binary CLI with `parse` / `serve` / `version` subcommands, then add goreleaser + GitHub Actions to publish it via a personal Homebrew tap.

**Architecture:** Move `cmd/server/` → `cmd/sharelink/` with subcommand dispatch via `flag.NewFlagSet`. Extract a testable `run(args, stdin, stdout, stderr) int` entry point so subcommands can be unit-tested with stubbed extractors. Teach `douyin.Service` to tolerate `nil` TTWidManager so CLI doesn't pay the ttwid round-trip by default. Add `.goreleaser.yaml` + `.github/workflows/release.yml` for tag-triggered cross-platform builds and auto-published brew formula.

**Tech Stack:** Go stdlib only (no cobra). goreleaser v2 for release pipeline. GitHub Actions runner. Homebrew tap (separate repo).

---

## File Structure

| Path | New / Modified / Deleted | Purpose |
|------|---------------------------|---------|
| `cmd/sharelink/main.go` | New | `os.Exit(run(...))` |
| `cmd/sharelink/run.go` | New | `run(args, stdin, stdout, stderr) int` — subcommand router |
| `cmd/sharelink/parse.go` | New | `runParse` + `readInput` + `realExtractorFactory` + `writeJSON` |
| `cmd/sharelink/serve.go` | New | `runServe` (moved/adapted from `cmd/server/main.go`) |
| `cmd/sharelink/version.go` | New | `runVersion` + ldflag-injected `version`/`commit`/`date` |
| `cmd/sharelink/parse_test.go` | New | argv / stdin / `--json` / errors |
| `cmd/sharelink/run_test.go` | New | router-level tests (unknown command, no command, version) |
| `cmd/server/main.go` | Deleted | Replaced by `cmd/sharelink/` |
| `cmd/server/doc.go` | Deleted (if present) | Same |
| `internal/douyin/service.go` | Modified | Tolerate `nil` ttwid via private helpers |
| `internal/douyin/service_test.go` | Modified | 2 new tests for nil ttwid |
| `.goreleaser.yaml` | New | Cross-platform build + brew formula publish |
| `.github/workflows/release.yml` | New | Tag-triggered goreleaser run |
| `LICENSE` | New | MIT, copyright procmeans 2026 |
| `README.md` | Modified | brew install + CLI usage; `serve` instead of `./server` |

---

## Task 1: `Service` tolerates nil TTWidManager

**Files:**
- Modify: `internal/douyin/service.go`
- Modify: `internal/douyin/service_test.go`

- [ ] **Step 1: Add failing tests for nil ttwid**

Append to `internal/douyin/service_test.go`:

```go
func TestService_NilTTWid_HappyPath(t *testing.T) {
	fixture := mustReadFixture(t)
	srv, shortURL, shareCalls := fakeDouyin(t, "7234567890123456789", [][]byte{fixture})
	defer srv.Close()

	svc := NewService(nil, cache.New(10, time.Minute))
	res, err := svc.ExtractWithConfig(Config{ShortURL: shortURL, ShareBaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.PlayURL == "" {
		t.Fatal("want non-empty play_url")
	}
	if res.VideoID != "7234567890123456789" {
		t.Fatalf("want video id, got %q", res.VideoID)
	}
	if *shareCalls != 1 {
		t.Fatalf("want 1 share call, got %d", *shareCalls)
	}
}

func TestService_NilTTWid_NoPlayURL_NoRetry(t *testing.T) {
	emptyShare := []byte(`<html><body>nothing</body></html>`)
	srv, shortURL, shareCalls := fakeDouyin(t, "7234567890123456789", [][]byte{emptyShare})
	defer srv.Close()

	svc := NewService(nil, cache.New(10, time.Minute))
	_, err := svc.ExtractWithConfig(Config{ShortURL: shortURL, ShareBaseURL: srv.URL})
	if !errors.Is(err, ErrNoPlayURL) {
		t.Fatalf("want ErrNoPlayURL, got %v", err)
	}
	if *shareCalls != 1 {
		t.Fatalf("nil ttwid should NOT retry, want 1 share call, got %d", *shareCalls)
	}
}
```

- [ ] **Step 2: Verify they fail**

```bash
go test ./internal/douyin/ -run 'TestService_NilTTWid' -count=1
```

Expected: panic / nil-pointer dereference inside `s.ttwid.Cookie()`.

- [ ] **Step 3: Add nil-safe helpers and rewire `ExtractWithConfig`**

Edit `internal/douyin/service.go`. Add two private helpers below the constructor:

```go
func (s *Service) cookie() string {
	if s.ttwid == nil {
		return ""
	}
	return s.ttwid.Cookie()
}

func (s *Service) refresh() error {
	if s.ttwid == nil {
		return errors.New("ttwid manager not configured")
	}
	return s.ttwid.Refresh()
}
```

Replace every `s.ttwid.Cookie()` call in `ExtractWithConfig` with `s.cookie()`, and every `s.ttwid.Refresh()` with `s.refresh()`. The full new body:

```go
func (s *Service) ExtractWithConfig(cfg Config) (Result, error) {
	if cfg.ShareBaseURL == "" {
		cfg.ShareBaseURL = shareBaseURL
	}

	videoID, err := ResolveVideoID(httpclient.NewNoRedirect(), s.cookie(), cfg.ShortURL)
	if err != nil {
		return Result{}, err
	}

	if cached, ok := s.cache.Get(videoID); ok {
		return Result{PlayURL: cached, VideoID: videoID, CacheHit: true}, nil
	}

	shareClient := httpclient.NewFollowing()
	playWM, err := fetchPlayURLFromBase(shareClient, s.cookie(), cfg.ShareBaseURL, videoID)
	refreshed := false
	if errors.Is(err, ErrNoPlayURL) {
		refreshed = true
		if rerr := s.refresh(); rerr != nil {
			return Result{VideoID: videoID, TTWidRefreshed: refreshed}, err
		}
		playWM, err = fetchPlayURLFromBase(shareClient, s.cookie(), cfg.ShareBaseURL, videoID)
	}
	if err != nil {
		return Result{VideoID: videoID, TTWidRefreshed: refreshed}, err
	}

	playURL := StripWatermark(playWM)
	s.cache.Set(videoID, playURL)
	return Result{PlayURL: playURL, VideoID: videoID, TTWidRefreshed: refreshed}, nil
}
```

- [ ] **Step 4: Verify all service tests pass**

```bash
go test ./internal/douyin/ -count=1 -race
```

Expected: PASS for everything (existing service tests + 2 new nil-ttwid tests).

- [ ] **Step 5: Commit**

```bash
git add internal/douyin/service.go internal/douyin/service_test.go
git commit -m "feat(douyin): tolerate nil TTWidManager in Service"
```

---

## Task 2: Scaffold `cmd/sharelink/` with version subcommand

**Files:**
- Create: `cmd/sharelink/main.go`
- Create: `cmd/sharelink/run.go`
- Create: `cmd/sharelink/version.go`
- Create: `cmd/sharelink/run_test.go`

This task lays down the router skeleton and the simplest subcommand (`version`). `parse` and `serve` come in later tasks but are dispatched as stubs returning code 1 here so the router compiles.

- [ ] **Step 1: Write router tests**

Create `cmd/sharelink/run_test.go`:

```go
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
```

- [ ] **Step 2: Verify tests fail**

```bash
go test ./cmd/sharelink/
```

Expected: compile error (`run`, `version` undefined).

- [ ] **Step 3: Implement `main.go`**

Create `cmd/sharelink/main.go`:

```go
// Package main is the sharelink CLI entrypoint.
package main

import "os"

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}
```

- [ ] **Step 4: Implement `run.go` with stubs for parse/serve**

Create `cmd/sharelink/run.go`:

```go
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
```

- [ ] **Step 5: Implement `version.go`**

Create `cmd/sharelink/version.go`:

```go
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
```

- [ ] **Step 6: Verify tests pass**

```bash
go test ./cmd/sharelink/ -v
go build ./...
```

Expected: all 4 router tests PASS; build succeeds.

- [ ] **Step 7: Commit**

```bash
git add cmd/sharelink/main.go cmd/sharelink/run.go cmd/sharelink/version.go cmd/sharelink/run_test.go
git commit -m "feat(cmd/sharelink): scaffold CLI with version subcommand"
```

---

## Task 3: `parse` subcommand

**Files:**
- Create: `cmd/sharelink/parse.go`
- Create: `cmd/sharelink/parse_test.go`
- Modify: `cmd/sharelink/run.go` (wire up real `runParse`)

- [ ] **Step 1: Write parse tests**

Create `cmd/sharelink/parse_test.go`:

```go
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
```

- [ ] **Step 2: Verify tests fail**

```bash
go test ./cmd/sharelink/ -run TestRunParse
```

Expected: compile error (`runParse`, `extractorFactory` undefined).

- [ ] **Step 3: Implement `parse.go`**

Create `cmd/sharelink/parse.go`:

```go
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/procmeans/sharelink/internal/cache"
	"github.com/procmeans/sharelink/internal/douyin"
	"github.com/procmeans/sharelink/internal/httpclient"
)

// extractorFactory returns an extract function given the --with-ttwid flag.
// The real factory builds a Service; tests inject stubs.
type extractorFactory func(withTTWid bool) (func(input string) (douyin.Result, error), error)

func runParse(args []string, stdin io.Reader, stdout, stderr io.Writer, mkExtractor extractorFactory) int {
	fs := flag.NewFlagSet("parse", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "output JSON instead of plain URL")
	withTTWid := fs.Bool("with-ttwid", false, "fetch ttwid cookie before parsing")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	input, err := readInput(fs.Args(), stdin)
	if err != nil {
		fmt.Fprintln(stderr, "sharelink:", err)
		return 2
	}

	extract, err := mkExtractor(*withTTWid)
	if err != nil {
		fmt.Fprintln(stderr, "sharelink:", err)
		return 1
	}

	res, err := extract(input)
	if err != nil {
		fmt.Fprintln(stderr, "sharelink:", err)
		return 1
	}

	if *jsonOut {
		return writeJSON(stdout, stderr, res)
	}
	fmt.Fprintln(stdout, res.PlayURL)
	return 0
}

// readInput returns input from extraArgs[0] if any, else reads all of stdin.
func readInput(extraArgs []string, stdin io.Reader) (string, error) {
	if len(extraArgs) > 0 {
		return strings.TrimSpace(extraArgs[0]), nil
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", errors.New("no input on argv or stdin")
	}
	return s, nil
}

func writeJSON(stdout, stderr io.Writer, res douyin.Result) int {
	out := struct {
		PlayURL string `json:"play_url"`
		VideoID string `json:"video_id"`
	}{
		PlayURL: res.PlayURL,
		VideoID: res.VideoID,
	}
	b, err := json.Marshal(out)
	if err != nil {
		fmt.Fprintln(stderr, "sharelink:", err)
		return 1
	}
	fmt.Fprintln(stdout, string(b))
	return 0
}

// realExtractorFactory constructs a Service with a no-op cache and optional
// ttwid manager, then returns the bound Extract method.
func realExtractorFactory(withTTWid bool) (func(string) (douyin.Result, error), error) {
	c := cache.New(0, 0)
	var ttwid *douyin.TTWidManager
	if withTTWid {
		ttwid = douyin.NewTTWidManager(httpclient.NewNoRedirect())
		if err := ttwid.Refresh(); err != nil {
			return nil, fmt.Errorf("ttwid refresh: %w", err)
		}
	}
	return douyin.NewService(ttwid, c).Extract, nil
}
```

- [ ] **Step 4: Wire `run.go` to call `runParse`**

In `cmd/sharelink/run.go`, replace the `case "parse":` stub block with:

```go
	case "parse":
		return runParse(args[2:], stdin, stdout, stderr, realExtractorFactory)
```

- [ ] **Step 5: Verify all tests pass**

```bash
go test ./cmd/sharelink/ -v -count=1
```

Expected: 4 router tests + 6 parse tests = 10 PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/sharelink/parse.go cmd/sharelink/parse_test.go cmd/sharelink/run.go
git commit -m "feat(cmd/sharelink): add parse subcommand"
```

---

## Task 4: `serve` subcommand — move from `cmd/server/`

**Files:**
- Create: `cmd/sharelink/serve.go`
- Modify: `cmd/sharelink/run.go` (wire real `runServe`)
- Delete: `cmd/server/main.go` (and `cmd/server/doc.go` if it still exists)

- [ ] **Step 1: Read current `cmd/server/main.go` to know what we're moving**

```bash
cat cmd/server/main.go
```

This is the source of truth for `runServe`. The new `serve.go` is a small adaptation: it takes `args []string` to support `--port`, but otherwise replicates the same startup sequence.

- [ ] **Step 2: Create `cmd/sharelink/serve.go`**

```go
package main

import (
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/procmeans/sharelink/internal/cache"
	"github.com/procmeans/sharelink/internal/douyin"
	"github.com/procmeans/sharelink/internal/httpapi"
	"github.com/procmeans/sharelink/internal/httpclient"
)

func runServe(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	portFlag := fs.String("port", "", "listen port (defaults to $PORT or 8080)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	ttwid := douyin.NewTTWidManager(httpclient.NewNoRedirect())
	if err := ttwid.Refresh(); err != nil {
		slog.Error("ttwid refresh failed at startup", "err", err.Error())
		return 1
	}
	slog.Info("ttwid acquired", "len", len(ttwid.Cookie()))

	c := cache.New(1000, 30*time.Minute)
	svc := douyin.NewService(ttwid, c)

	port := *portFlag
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      httpapi.NewHandler(svc.Extract),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	slog.Info("server starting", "addr", srv.Addr)
	err := srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "err", err)
		return 1
	}
	return 0
}
```

- [ ] **Step 3: Wire `run.go` to call `runServe`**

In `cmd/sharelink/run.go`, replace the `case "serve":` stub block with:

```go
	case "serve":
		return runServe(args[2:], stderr)
```

- [ ] **Step 4: Delete the old `cmd/server/` directory**

```bash
rm -rf cmd/server/
```

- [ ] **Step 5: Build and smoke-test**

```bash
go build -o sharelink ./cmd/sharelink
./sharelink
echo "exit=$?"
./sharelink wat 2>&1
echo "exit=$?"
./sharelink version
```

Expected:
- Empty subcommand: prints usage, exit code 2.
- `wat`: prints "unknown command", exit 2.
- `version`: prints `sharelink dev (commit none, built unknown)`, exit 0.

Then smoke-test `serve` (needs network):

```bash
PORT=18080 ./sharelink serve > /tmp/sharelink.log 2>&1 &
SERVER_PID=$!
sleep 2
if kill -0 $SERVER_PID 2>/dev/null; then
  curl -fsS http://localhost:18080/healthz
  echo
  kill $SERVER_PID
  wait $SERVER_PID 2>/dev/null
else
  echo "serve died:"
  cat /tmp/sharelink.log
  exit 1
fi
```

Expected: `ok` printed.

- [ ] **Step 6: Run full test suite**

```bash
go test ./... -race -count=1
go vet ./...
```

Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add cmd/sharelink/serve.go cmd/sharelink/run.go
git rm -r cmd/server/
git commit -m "feat(cmd/sharelink): add serve subcommand, drop cmd/server"
```

---

## Task 5: LICENSE + README

**Files:**
- Create: `LICENSE`
- Modify: `README.md`

- [ ] **Step 1: Create LICENSE**

Create `LICENSE` with the standard MIT text:

```
MIT License

Copyright (c) 2026 procmeans <procmeans@proton.me>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 2: Rewrite README.md**

Replace the contents of `README.md` with:

````markdown
# sharelink

Extract watermark-free direct video URLs from Douyin (抖音) share text or short links.

## Install

```bash
brew tap procmeans/tap
brew install sharelink
```

Or build from source:

```bash
go install github.com/procmeans/sharelink/cmd/sharelink@latest
```

## CLI

### Parse a share link

```bash
sharelink parse "5.99 fyu:/ 复制打开抖音 https://v.douyin.com/iABCxxx/ ..."
# https://aweme.snssdk.com/aweme/v1/play/?video_id=...
```

Or pipe from clipboard:

```bash
pbpaste | sharelink parse
```

JSON output:

```bash
sharelink parse --json "https://v.douyin.com/iABCxxx/"
# {"play_url":"https://...","video_id":"7234567890123456789"}
```

If you hit `2002`-style errors, try `--with-ttwid` to fetch a cookie first (~+500ms):

```bash
sharelink parse --with-ttwid "https://v.douyin.com/iABCxxx/"
```

Exit codes: `0` success, `1` parse/upstream error, `2` usage error.

### Run as HTTP service

```bash
sharelink serve              # listens on $PORT or :8080
sharelink serve --port 9090
```

#### `POST /api/parse`

```json
{ "url": "5.99 fyu:/ ... https://v.douyin.com/iABCxyz/ ..." }
```

Success response:

```json
{ "code": 0, "play_url": "https://..." }
```

Error response:

```json
{ "code": 2001, "message": "douyin: upstream request failed: ..." }
```

#### Error codes

| code | HTTP | meaning |
|------|------|---------|
| 0    | 200  | success |
| 1001 | 400  | bad request body |
| 1002 | 422  | no v.douyin.com short URL in input |
| 1003 | 422  | redirect target had no video id |
| 1004 | 405  | method not allowed |
| 2001 | 502  | upstream request failed |
| 2002 | 502  | RENDER_DATA missing or play URL empty |

#### `GET /healthz`

Returns `200 ok`. For health checks.

### Version

```bash
sharelink version
# sharelink v0.1.0 (commit abc1234, built 2026-05-19T...)
```

## Operational notes (serve mode)

- On startup the server fetches a `ttwid` cookie from `https://www.douyin.com/`. Failure is fatal.
- Successful parses are cached in-process by `video_id` for 30 minutes (LRU, max 1000 entries).
- If the share-page parser returns "no play URL" the server refreshes ttwid and retries once.
- Each request log includes `cache_hit` and `ttwid_refreshed` fields.

## Build and test

```bash
go test ./... -race
go vet ./...
go build -o sharelink ./cmd/sharelink
```

All tests are offline; no real Douyin requests.

## Release (maintainer)

A push of a `v*.*.*` tag triggers `.github/workflows/release.yml`, which runs goreleaser to build cross-platform binaries and publishes a brew formula to `procmeans/homebrew-tap`. See `docs/superpowers/specs/2026-05-19-cli-and-brew-design.md` §6.3 for the one-time setup (creating the tap repo and `TAP_GITHUB_TOKEN` secret).

```bash
git tag v0.1.0
git push origin v0.1.0
```
````

- [ ] **Step 3: Commit**

```bash
git add LICENSE README.md
git commit -m "docs: add LICENSE and CLI-focused README"
```

---

## Task 6: goreleaser config

**Files:**
- Create: `.goreleaser.yaml`

- [ ] **Step 1: Create the config**

Create `.goreleaser.yaml`:

```yaml
version: 2
project_name: sharelink

builds:
  - id: sharelink
    main: ./cmd/sharelink
    binary: sharelink
    env:
      - CGO_ENABLED=0
    goos: [darwin, linux]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}

archives:
  - id: sharelink
    format: tar.gz
    name_template: '{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}'
    files:
      - LICENSE
      - README.md

checksum:
  name_template: 'checksums.txt'

changelog:
  sort: asc
  filters:
    exclude:
      - '^docs:'
      - '^test:'
      - '^chore:'

brews:
  - name: sharelink
    repository:
      owner: procmeans
      name: homebrew-tap
      token: '{{ .Env.TAP_GITHUB_TOKEN }}'
    directory: Formula
    homepage: https://github.com/procmeans/sharelink
    description: Douyin watermark-free direct link extractor
    license: MIT
    test: |
      system "#{bin}/sharelink", "version"
    install: |
      bin.install "sharelink"
```

- [ ] **Step 2: Validate locally (if goreleaser is installed)**

```bash
goreleaser check 2>&1 || echo "goreleaser not installed locally — that's OK, CI will validate"
```

Expected: either "config is valid" or a message saying goreleaser isn't installed. **The config does not need goreleaser to be installed locally; CI will run it.** If `goreleaser check` IS run and reports an error, fix the YAML inline and re-run.

- [ ] **Step 3: Dry-run snapshot build (optional, only if goreleaser is installed locally)**

```bash
goreleaser release --snapshot --clean --skip=publish 2>&1 || echo "skipped — goreleaser not installed"
```

Expected: if it ran, it produced binaries under `dist/`. If skipped, that's fine — we trust CI to catch real issues.

- [ ] **Step 4: Commit**

```bash
git add .goreleaser.yaml
git commit -m "build: add goreleaser config"
```

---

## Task 7: GitHub Actions release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Create the workflow**

Create `.github/workflows/release.yml`:

```yaml
name: release

on:
  push:
    tags: ['v*.*.*']

permissions:
  contents: write

jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - uses: goreleaser/goreleaser-action@v6
        with:
          version: '~> v2'
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          TAP_GITHUB_TOKEN: ${{ secrets.TAP_GITHUB_TOKEN }}
```

- [ ] **Step 2: Verify the YAML parses**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml')); print('OK')"
```

Expected: prints `OK`.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add tag-triggered goreleaser workflow"
```

---

## Task 8: Final acceptance

**Files:** (no new files — just verification)

- [ ] **Step 1: Full test suite with race detector**

```bash
go test ./... -race -count=1
```

Expected: all packages green (cache, douyin, httpapi, cmd/sharelink).

- [ ] **Step 2: vet**

```bash
go vet ./...
```

Expected: no output.

- [ ] **Step 3: Build binary**

```bash
go build -o sharelink ./cmd/sharelink
ls -la sharelink
```

Expected: binary exists, ~9MB.

- [ ] **Step 4: Manual CLI smoke tests (no network needed)**

```bash
# 4a. No subcommand → usage on stderr, exit 2
./sharelink 2>&1 >/dev/null | head -3
echo "exit=$?"

# 4b. Unknown subcommand → exit 2
./sharelink wat 2>&1
echo "exit=$?"

# 4c. version → exit 0, contains "dev"
./sharelink version
echo "exit=$?"

# 4d. help
./sharelink -h | head -3

# 4e. parse with no input → exit 2
echo "" | ./sharelink parse
echo "exit=$?"
```

Expected outputs:
- 4a: "Usage:" line; exit 2.
- 4b: "unknown command"; exit 2.
- 4c: "sharelink dev (commit none, built unknown)"; exit 0.
- 4d: "Usage:" lines printed.
- 4e: "sharelink: no input on argv or stdin"; exit 2.

- [ ] **Step 5: Real-link smoke test (requires network)**

```bash
echo "https://v.douyin.com/R2mGwrMjlLw/" | ./sharelink parse
```

Expected: one line containing a URL with `play` in it (not `playwm`), no leading/trailing junk. Exit 0.

Then:

```bash
./sharelink parse --json "https://v.douyin.com/R2mGwrMjlLw/"
```

Expected: one line of JSON like `{"play_url":"https://...","video_id":"7..."}`.

If either fails, STOP and report.

- [ ] **Step 6: Real-link serve smoke test**

```bash
PORT=18080 ./sharelink serve > /tmp/sharelink.log 2>&1 &
SERVER_PID=$!
sleep 2
if ! kill -0 $SERVER_PID 2>/dev/null; then
  echo "serve died on startup:"
  cat /tmp/sharelink.log
  exit 1
fi
curl -fsS http://localhost:18080/healthz
echo
curl -s -X POST http://localhost:18080/api/parse \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://v.douyin.com/R2mGwrMjlLw/"}' | python3 -m json.tool
kill $SERVER_PID
wait 2>/dev/null
grep '"msg":"ttwid acquired"' /tmp/sharelink.log
```

Expected:
- `/healthz` returns `ok`.
- `/api/parse` returns `{"code": 0, "play_url": "..."}`.
- Log contains the ttwid-acquired line.

- [ ] **Step 7: Commit (only if anything changed since Task 7)**

Run `git status`. If there are no changes, skip this step.

```bash
git status
# If clean, no commit. Otherwise:
# git add ... && git commit -m "..."
```

---

## Out of Scope (Reminder)

Per spec §1, these are intentionally NOT in this plan:
- Shell completion (bash/zsh/fish)
- `--config <file>` support
- Snapshot / nightly release channels (only `v*.*.*` tags trigger releases)
- Linux package managers other than brew (deb/rpm/apt/aur)
- Windows binaries
- Automated changelog beyond goreleaser's built-in
- The one-time GitHub setup steps from spec §6.3 (creating `procmeans/sharelink` + `procmeans/homebrew-tap` repos, generating `TAP_GITHUB_TOKEN` PAT, configuring the secret) — these are manual maintainer tasks documented in the README, not implementation work.
