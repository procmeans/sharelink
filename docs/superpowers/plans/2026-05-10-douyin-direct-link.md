# 抖音去水印直链解析服务 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go HTTP service that accepts Douyin share text/short links and returns the watermark-free mp4 CDN URL of the video.

**Architecture:** Standard-library Go HTTP server. Pure-logic `internal/douyin` package extracts the short URL → follows the 302 to get a `video_id` → fetches `iesdouyin.com/share/video/{id}/` → parses the embedded `RENDER_DATA` JSON → strips `playwm` → `play` from the play URL. `internal/httpapi` wraps it as `POST /api/parse`. No external dependencies, no caching, no retries.

**Tech Stack:** Go 1.22+, standard library only (`net/http`, `encoding/json`, `regexp`, `net/url`, `log/slog`).

---

## File Structure

Files created by this plan:

| Path | Purpose |
|------|---------|
| `go.mod` | Module declaration |
| `cmd/server/main.go` | Process entry: read PORT, build HTTP server, wire handler |
| `internal/httpclient/client.go` | Two preconfigured `*http.Client` (one follows redirects, one doesn't) + UA constant |
| `internal/douyin/errors.go` | Sentinel errors |
| `internal/douyin/shorturl.go` | `ExtractShortURL(text string) (string, error)` |
| `internal/douyin/shorturl_test.go` | Table-driven tests |
| `internal/douyin/resolve.go` | `ResolveVideoID(client, shortURL) (string, error)` |
| `internal/douyin/resolve_test.go` | Tests using `httptest.NewServer` |
| `internal/douyin/share.go` | `FetchPlayURL(client, videoID) (string, error)` — page fetch + RENDER_DATA parse |
| `internal/douyin/share_test.go` | Test against fixture HTML + httptest server |
| `internal/douyin/testdata/sample_share.html` | Fixture: a real captured share-page HTML |
| `internal/douyin/nowm.go` | `StripWatermark(url string) string` |
| `internal/douyin/nowm_test.go` | Table-driven tests |
| `internal/douyin/extract.go` | `Extract(input string) (string, error)` orchestrating the four steps |
| `internal/douyin/extract_test.go` | End-to-end test with httptest mocking 302 + share page |
| `internal/httpapi/handler.go` | Router, request/response types, error mapping |
| `internal/httpapi/handler_test.go` | `httptest.NewRecorder` based tests |

---

## Task 1: Initialize Go module and directory layout

**Files:**
- Create: `go.mod`
- Create: `cmd/server/`, `internal/douyin/`, `internal/httpapi/`, `internal/httpclient/` (empty dirs)

- [ ] **Step 1: Init module**

Run from repo root (`/Users/procmeans/Tools/sharelink`):

```bash
go mod init github.com/procmeans/sharelink
```

Expected: creates `go.mod` with `module github.com/procmeans/sharelink` and `go 1.22` (or current).

- [ ] **Step 2: Create package directories with placeholder doc files**

Create `cmd/server/doc.go`:

```go
// Package main is the HTTP server entrypoint for the Douyin direct-link service.
package main
```

Create `internal/douyin/doc.go`:

```go
// Package douyin extracts watermark-free CDN URLs from Douyin share links.
package douyin
```

Create `internal/httpapi/doc.go`:

```go
// Package httpapi exposes the douyin extractor as an HTTP service.
package httpapi
```

Create `internal/httpclient/doc.go`:

```go
// Package httpclient provides preconfigured HTTP clients for Douyin requests.
package httpclient
```

- [ ] **Step 3: Initialize git repo (if not already one)**

```bash
cd /Users/procmeans/Tools/sharelink
[ -d .git ] || git init
```

Create `.gitignore`:

```
# binaries
/server
/cmd/server/server

# editor
.DS_Store
.idea/
.vscode/
```

- [ ] **Step 4: Verify build compiles**

```bash
go build ./...
```

Expected: no output, no error.

- [ ] **Step 5: Commit**

```bash
git add go.mod .gitignore cmd internal docs
git commit -m "chore: scaffold project layout and module"
```

---

## Task 2: Sentinel errors

**Files:**
- Create: `internal/douyin/errors.go`

- [ ] **Step 1: Write `errors.go`**

```go
package douyin

import "errors"

var (
	// ErrNoShortURL means the input text contains no v.douyin.com link.
	ErrNoShortURL = errors.New("douyin: no short url found in input")

	// ErrInvalidRedirect means the 302 target did not contain a parseable video id.
	ErrInvalidRedirect = errors.New("douyin: redirect target has no video id")

	// ErrUpstream means an HTTP request to Douyin failed or returned a non-2xx status.
	ErrUpstream = errors.New("douyin: upstream request failed")

	// ErrNoPlayURL means the share page did not contain an extractable play URL.
	ErrNoPlayURL = errors.New("douyin: play url not found in share page")
)
```

- [ ] **Step 2: Build**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/douyin/errors.go
git commit -m "feat(douyin): add sentinel errors"
```

---

## Task 3: Short URL extraction

**Files:**
- Create: `internal/douyin/shorturl.go`
- Test: `internal/douyin/shorturl_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/douyin/shorturl_test.go`:

```go
package douyin

import (
	"errors"
	"testing"
)

func TestExtractShortURL(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{
			name:  "bare short url",
			input: "https://v.douyin.com/iABCxyz/",
			want:  "https://v.douyin.com/iABCxyz/",
		},
		{
			name:  "share text with emoji and trailing slash",
			input: "5.99 fyu:/ 复制打开抖音,看看【某某的作品】https://v.douyin.com/iABCxyz/ 复制此链接",
			want:  "https://v.douyin.com/iABCxyz/",
		},
		{
			name:  "http scheme",
			input: "look at this http://v.douyin.com/abc123 cool",
			want:  "http://v.douyin.com/abc123",
		},
		{
			name:  "no trailing slash",
			input: "see https://v.douyin.com/abc123 here",
			want:  "https://v.douyin.com/abc123",
		},
		{
			name:    "empty",
			input:   "",
			wantErr: ErrNoShortURL,
		},
		{
			name:    "no douyin url",
			input:   "this is just text https://example.com/foo",
			wantErr: ErrNoShortURL,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractShortURL(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want err %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}
```

- [ ] **Step 2: Verify test fails**

```bash
go test ./internal/douyin/ -run TestExtractShortURL
```

Expected: compile error `undefined: ExtractShortURL`.

- [ ] **Step 3: Implement**

Create `internal/douyin/shorturl.go`:

```go
package douyin

import "regexp"

var shortURLRe = regexp.MustCompile(`https?://v\.douyin\.com/[A-Za-z0-9]+/?`)

// ExtractShortURL finds the first v.douyin.com short URL in input.
// Returns ErrNoShortURL if none is present.
func ExtractShortURL(input string) (string, error) {
	match := shortURLRe.FindString(input)
	if match == "" {
		return "", ErrNoShortURL
	}
	return match, nil
}
```

- [ ] **Step 4: Verify test passes**

```bash
go test ./internal/douyin/ -run TestExtractShortURL -v
```

Expected: PASS for all subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/douyin/shorturl.go internal/douyin/shorturl_test.go
git commit -m "feat(douyin): extract v.douyin.com short url from text"
```

---

## Task 4: Watermark stripping

**Files:**
- Create: `internal/douyin/nowm.go`
- Test: `internal/douyin/nowm_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/douyin/nowm_test.go`:

```go
package douyin

import "testing"

func TestStripWatermark(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "playwm in path",
			in:   "https://aweme.snssdk.com/aweme/v1/playwm/?video_id=v01",
			want: "https://aweme.snssdk.com/aweme/v1/play/?video_id=v01",
		},
		{
			name: "already no watermark",
			in:   "https://aweme.snssdk.com/aweme/v1/play/?video_id=v01",
			want: "https://aweme.snssdk.com/aweme/v1/play/?video_id=v01",
		},
		{
			name: "only first occurrence is replaced",
			in:   "https://example.com/playwm/?ref=playwm",
			want: "https://example.com/play/?ref=playwm",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StripWatermark(tc.in)
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}
```

- [ ] **Step 2: Verify test fails**

```bash
go test ./internal/douyin/ -run TestStripWatermark
```

Expected: compile error `undefined: StripWatermark`.

- [ ] **Step 3: Implement**

Create `internal/douyin/nowm.go`:

```go
package douyin

import "strings"

// StripWatermark replaces the first "playwm" segment in url with "play",
// converting a watermarked Douyin CDN URL into a watermark-free one.
// If "playwm" is not present, the URL is returned unchanged.
func StripWatermark(url string) string {
	return strings.Replace(url, "playwm", "play", 1)
}
```

- [ ] **Step 4: Verify test passes**

```bash
go test ./internal/douyin/ -run TestStripWatermark -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/douyin/nowm.go internal/douyin/nowm_test.go
git commit -m "feat(douyin): strip playwm to play in cdn url"
```

---

## Task 5: HTTP client package

**Files:**
- Create: `internal/httpclient/client.go`

- [ ] **Step 1: Write `client.go`**

```go
package httpclient

import (
	"net/http"
	"time"
)

// MobileUA is an iPhone Safari user-agent. iesdouyin.com returns the share
// page reliably for mobile UAs; PC UAs sometimes get a different shell.
const MobileUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) " +
	"AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1"

const defaultTimeout = 8 * time.Second

// NewFollowing returns a client that follows redirects normally.
func NewFollowing() *http.Client {
	return &http.Client{
		Timeout: defaultTimeout,
	}
}

// NewNoRedirect returns a client that does NOT follow redirects;
// the caller is expected to read Location from the response.
func NewNoRedirect() *http.Client {
	return &http.Client{
		Timeout: defaultTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
```

- [ ] **Step 2: Build**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/httpclient/client.go
git commit -m "feat(httpclient): add following and no-redirect clients with mobile UA"
```

---

## Task 6: Resolve short URL → video_id

**Files:**
- Create: `internal/douyin/resolve.go`
- Test: `internal/douyin/resolve_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/douyin/resolve_test.go`:

```go
package douyin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/procmeans/sharelink/internal/httpclient"
)

func TestResolveVideoID(t *testing.T) {
	t.Run("share/video target", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "https://www.iesdouyin.com/share/video/7234567890123456789/?region=CN")
			w.WriteHeader(http.StatusFound)
		}))
		defer srv.Close()

		got, err := ResolveVideoID(httpclient.NewNoRedirect(), srv.URL)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != "7234567890123456789" {
			t.Fatalf("want video id, got %q", got)
		}
	})

	t.Run("www.douyin.com/video target", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "https://www.douyin.com/video/7111111111111111111")
			w.WriteHeader(http.StatusMovedPermanently)
		}))
		defer srv.Close()

		got, err := ResolveVideoID(httpclient.NewNoRedirect(), srv.URL)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != "7111111111111111111" {
			t.Fatalf("want video id, got %q", got)
		}
	})

	t.Run("location without video id", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "https://www.douyin.com/")
			w.WriteHeader(http.StatusFound)
		}))
		defer srv.Close()

		_, err := ResolveVideoID(httpclient.NewNoRedirect(), srv.URL)
		if !errors.Is(err, ErrInvalidRedirect) {
			t.Fatalf("want ErrInvalidRedirect, got %v", err)
		}
	})

	t.Run("upstream 500", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := ResolveVideoID(httpclient.NewNoRedirect(), srv.URL)
		if !errors.Is(err, ErrUpstream) {
			t.Fatalf("want ErrUpstream, got %v", err)
		}
	})
}
```

- [ ] **Step 2: Verify test fails**

```bash
go test ./internal/douyin/ -run TestResolveVideoID
```

Expected: compile error `undefined: ResolveVideoID`.

- [ ] **Step 3: Implement**

Create `internal/douyin/resolve.go`:

```go
package douyin

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/procmeans/sharelink/internal/httpclient"
)

var videoIDRe = regexp.MustCompile(`(?:share/video|video)/(\d+)`)

// ResolveVideoID GETs shortURL with a no-redirect client, reads the Location
// header from the 3xx response, and extracts the numeric video id.
func ResolveVideoID(client *http.Client, shortURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, shortURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", httpclient.MobileUA)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: do request: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("%w: unexpected status %d", ErrUpstream, resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("%w: missing Location header", ErrInvalidRedirect)
	}

	m := videoIDRe.FindStringSubmatch(loc)
	if len(m) < 2 {
		return "", fmt.Errorf("%w: location %q", ErrInvalidRedirect, loc)
	}
	return m[1], nil
}
```

- [ ] **Step 4: Verify test passes**

```bash
go test ./internal/douyin/ -run TestResolveVideoID -v
```

Expected: PASS for all subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/douyin/resolve.go internal/douyin/resolve_test.go
git commit -m "feat(douyin): resolve short url to video id via 302 location"
```

---

## Task 7: Share-page fixture

**Files:**
- Create: `internal/douyin/testdata/sample_share.html`

This task creates a synthetic but structurally faithful fixture so the next task's tests can run offline.

- [ ] **Step 1: Create fixture**

Create `internal/douyin/testdata/sample_share.html`. The file is a small HTML document containing a `<script id="RENDER_DATA">` whose content is the URL-encoded form of this JSON (note: the actual fixture must be one continuous URL-encoded string with NO whitespace inside the script tag):

JSON before URL-encoding:

```json
{"app":{"aweme_detail":{"aweme_id":"7234567890123456789","desc":"test video","video":{"play_addr":{"uri":"v0200fg10000abc","url_list":["https://aweme.snssdk.com/aweme/v1/playwm/?video_id=v0200fg10000abc&line=0","https://api.amemv.com/aweme/v1/playwm/?video_id=v0200fg10000abc&line=1"]}}}}}
```

Use Python to URL-encode and write the file (run this from the repo root):

```bash
python3 - <<'PY'
import urllib.parse, pathlib, json
data = {"app":{"aweme_detail":{"aweme_id":"7234567890123456789","desc":"test video","video":{"play_addr":{"uri":"v0200fg10000abc","url_list":["https://aweme.snssdk.com/aweme/v1/playwm/?video_id=v0200fg10000abc&line=0","https://api.amemv.com/aweme/v1/playwm/?video_id=v0200fg10000abc&line=1"]}}}}}
encoded = urllib.parse.quote(json.dumps(data, ensure_ascii=False, separators=(",", ":")))
html = f'<!doctype html><html><body><script id="RENDER_DATA" type="application/json">{encoded}</script></body></html>'
pathlib.Path("internal/douyin/testdata").mkdir(parents=True, exist_ok=True)
pathlib.Path("internal/douyin/testdata/sample_share.html").write_text(html, encoding="utf-8")
PY
```

- [ ] **Step 2: Verify fixture exists and is non-empty**

```bash
test -s internal/douyin/testdata/sample_share.html && echo OK
```

Expected: prints `OK`.

- [ ] **Step 3: Commit**

```bash
git add internal/douyin/testdata/sample_share.html
git commit -m "test(douyin): add share page fixture"
```

---

## Task 8: Share-page fetch + RENDER_DATA parse

**Files:**
- Create: `internal/douyin/share.go`
- Test: `internal/douyin/share_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/douyin/share_test.go`:

```go
package douyin

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/procmeans/sharelink/internal/httpclient"
)

func TestFetchPlayURL_Success(t *testing.T) {
	body, err := os.ReadFile("testdata/sample_share.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/share/video/7234567890123456789/") {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		if ua := r.Header.Get("User-Agent"); ua != httpclient.MobileUA {
			t.Errorf("want mobile UA, got %q", ua)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, string(body))
	}))
	defer srv.Close()

	got, err := fetchPlayURLFromBase(httpclient.NewFollowing(), srv.URL, "7234567890123456789")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "https://aweme.snssdk.com/aweme/v1/playwm/?video_id=v0200fg10000abc&line=0"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestFetchPlayURL_NoRenderData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "<html><body>nothing here</body></html>")
	}))
	defer srv.Close()

	_, err := fetchPlayURLFromBase(httpclient.NewFollowing(), srv.URL, "123")
	if !errors.Is(err, ErrNoPlayURL) {
		t.Fatalf("want ErrNoPlayURL, got %v", err)
	}
}

func TestFetchPlayURL_EmptyURLList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// RENDER_DATA present but url_list empty
		const empty = `%7B%22app%22%3A%7B%22aweme_detail%22%3A%7B%22video%22%3A%7B%22play_addr%22%3A%7B%22url_list%22%3A%5B%5D%7D%7D%7D%7D%7D`
		_, _ = io.WriteString(w, `<script id="RENDER_DATA" type="application/json">`+empty+`</script>`)
	}))
	defer srv.Close()

	_, err := fetchPlayURLFromBase(httpclient.NewFollowing(), srv.URL, "123")
	if !errors.Is(err, ErrNoPlayURL) {
		t.Fatalf("want ErrNoPlayURL, got %v", err)
	}
}

func TestFetchPlayURL_Upstream5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := fetchPlayURLFromBase(httpclient.NewFollowing(), srv.URL, "123")
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("want ErrUpstream, got %v", err)
	}
}
```

- [ ] **Step 2: Verify test fails**

```bash
go test ./internal/douyin/ -run TestFetchPlayURL
```

Expected: compile error `undefined: fetchPlayURLFromBase`.

- [ ] **Step 3: Implement**

Create `internal/douyin/share.go`:

```go
package douyin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"

	"github.com/procmeans/sharelink/internal/httpclient"
)

const shareBaseURL = "https://www.iesdouyin.com"

var renderDataRe = regexp.MustCompile(`<script id="RENDER_DATA"[^>]*>([^<]+)</script>`)

// renderData captures the minimal subset of fields we need from the
// share-page RENDER_DATA payload. Extra fields are ignored.
type renderData struct {
	App struct {
		AwemeDetail struct {
			Video struct {
				PlayAddr struct {
					URLList []string `json:"url_list"`
				} `json:"play_addr"`
			} `json:"video"`
		} `json:"aweme_detail"`
	} `json:"app"`
}

// FetchPlayURL fetches the share page for videoID and returns the first
// (still-watermarked) play URL from RENDER_DATA. Use StripWatermark on the
// result to remove the watermark.
func FetchPlayURL(client *http.Client, videoID string) (string, error) {
	return fetchPlayURLFromBase(client, shareBaseURL, videoID)
}

func fetchPlayURLFromBase(client *http.Client, baseURL, videoID string) (string, error) {
	target := fmt.Sprintf("%s/share/video/%s/", baseURL, videoID)

	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", httpclient.MobileUA)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: do request: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: unexpected status %d", ErrUpstream, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: read body: %v", ErrUpstream, err)
	}

	m := renderDataRe.FindSubmatch(body)
	if len(m) < 2 {
		return "", fmt.Errorf("%w: RENDER_DATA script not found", ErrNoPlayURL)
	}

	decoded, err := url.QueryUnescape(string(m[1]))
	if err != nil {
		return "", fmt.Errorf("%w: decode RENDER_DATA: %v", ErrNoPlayURL, err)
	}

	var rd renderData
	if err := json.Unmarshal([]byte(decoded), &rd); err != nil {
		return "", fmt.Errorf("%w: unmarshal RENDER_DATA: %v", ErrNoPlayURL, err)
	}

	urls := rd.App.AwemeDetail.Video.PlayAddr.URLList
	if len(urls) == 0 || urls[0] == "" {
		return "", fmt.Errorf("%w: empty url_list", ErrNoPlayURL)
	}
	return urls[0], nil
}
```

- [ ] **Step 4: Verify test passes**

```bash
go test ./internal/douyin/ -run TestFetchPlayURL -v
```

Expected: PASS for all subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/douyin/share.go internal/douyin/share_test.go
git commit -m "feat(douyin): fetch share page and parse RENDER_DATA play url"
```

---

## Task 9: Orchestrator `Extract`

**Files:**
- Create: `internal/douyin/extract.go`
- Test: `internal/douyin/extract_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/douyin/extract_test.go`:

```go
package douyin

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// fakeDouyin simulates both the v.douyin.com short-URL 302 and the
// iesdouyin.com share page in a single httptest server. Routes:
//   GET /short/* → 302 to /share/video/<id>/
//   GET /share/video/<id>/ → returns fixture HTML
func fakeDouyin(t *testing.T, videoID, fixturePath string) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	mux := http.NewServeMux()
	srv := httptest.NewUnstartedServer(mux)

	mux.HandleFunc("/short/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", srv.URL+"/share/video/"+videoID+"/")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/share/video/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, string(body))
	})

	srv.Start()
	return srv
}

func TestExtract_EndToEnd(t *testing.T) {
	srv := fakeDouyin(t, "7234567890123456789", "testdata/sample_share.html")
	defer srv.Close()

	// Inject the test server as both the short-URL host and share-base.
	cfg := Config{
		ShortURL:        srv.URL + "/short/abc",
		ShareBaseURL:    srv.URL,
	}
	got, err := ExtractWithConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "https://aweme.snssdk.com/aweme/v1/play/?video_id=v0200fg10000abc&line=0"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestExtract_NoShortURL(t *testing.T) {
	_, err := Extract("hello world, no link here")
	if !errors.Is(err, ErrNoShortURL) {
		t.Fatalf("want ErrNoShortURL, got %v", err)
	}
}

func TestExtract_ShortURLEmbeddedInText(t *testing.T) {
	// Just verifies the input parser is wired in; no network needed because
	// the resolve step will fail on the bogus host. We only check the error
	// kind isn't ErrNoShortURL (the parse step succeeded).
	_, err := Extract("text https://v.douyin.com/iNotReal/ more text")
	if errors.Is(err, ErrNoShortURL) {
		t.Fatalf("short URL parse failed unexpectedly: %v", err)
	}
	// Network/DNS errors will surface as ErrUpstream — accept either that
	// or ErrInvalidRedirect depending on local DNS behaviour.
	if !errors.Is(err, ErrUpstream) && !errors.Is(err, ErrInvalidRedirect) {
		t.Fatalf("want ErrUpstream/ErrInvalidRedirect, got %v", err)
	}
}
```

- [ ] **Step 2: Verify test fails**

```bash
go test ./internal/douyin/ -run TestExtract
```

Expected: compile error `undefined: Extract` / `undefined: ExtractWithConfig` / `undefined: Config`.

- [ ] **Step 3: Implement**

Create `internal/douyin/extract.go`:

```go
package douyin

import (
	"github.com/procmeans/sharelink/internal/httpclient"
)

// Config lets tests inject a fake short URL and share base. Production code
// should call Extract(input) instead.
type Config struct {
	// ShortURL is the v.douyin.com short URL to resolve.
	ShortURL string
	// ShareBaseURL is the base URL for the share page (default: shareBaseURL).
	ShareBaseURL string
}

// Extract is the package's high-level entry point: take user-pasted share
// text or a short URL and return a watermark-free mp4 URL.
func Extract(input string) (string, error) {
	short, err := ExtractShortURL(input)
	if err != nil {
		return "", err
	}
	return ExtractWithConfig(Config{ShortURL: short})
}

// ExtractWithConfig runs the resolve→fetch→strip pipeline with an explicit
// short URL and (optional) share base. Used by Extract and tests.
func ExtractWithConfig(cfg Config) (string, error) {
	if cfg.ShareBaseURL == "" {
		cfg.ShareBaseURL = shareBaseURL
	}

	videoID, err := ResolveVideoID(httpclient.NewNoRedirect(), cfg.ShortURL)
	if err != nil {
		return "", err
	}

	playWM, err := fetchPlayURLFromBase(httpclient.NewFollowing(), cfg.ShareBaseURL, videoID)
	if err != nil {
		return "", err
	}

	return StripWatermark(playWM), nil
}
```

- [ ] **Step 4: Verify test passes**

```bash
go test ./internal/douyin/ -v
```

Expected: PASS for all tests in the package.

- [ ] **Step 5: Commit**

```bash
git add internal/douyin/extract.go internal/douyin/extract_test.go
git commit -m "feat(douyin): orchestrate end-to-end Extract pipeline"
```

---

## Task 10: HTTP handler

**Files:**
- Create: `internal/httpapi/handler.go`
- Test: `internal/httpapi/handler_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/httpapi/handler_test.go`:

```go
package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/procmeans/sharelink/internal/douyin"
)

func TestParseHandler_BadJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")

	NewHandler(stubExtract("", nil)).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	var got map[string]any
	mustDecode(t, rec.Body, &got)
	if int(got["code"].(float64)) != 1001 {
		t.Fatalf("want code 1001, got %v", got["code"])
	}
}

func TestParseHandler_MissingURL(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	NewHandler(stubExtract("", nil)).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestParseHandler_NoShortURL(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{"url":"hi"}`))
	req.Header.Set("Content-Type", "application/json")

	NewHandler(stubExtract("", douyin.ErrNoShortURL)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", rec.Code)
	}
	var got map[string]any
	mustDecode(t, rec.Body, &got)
	if int(got["code"].(float64)) != 1002 {
		t.Fatalf("want code 1002, got %v", got["code"])
	}
}

func TestParseHandler_InvalidRedirect(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/parse",
		strings.NewReader(`{"url":"https://v.douyin.com/x/"}`))
	req.Header.Set("Content-Type", "application/json")

	NewHandler(stubExtract("", douyin.ErrInvalidRedirect)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", rec.Code)
	}
	var got map[string]any
	mustDecode(t, rec.Body, &got)
	if int(got["code"].(float64)) != 1003 {
		t.Fatalf("want code 1003, got %v", got["code"])
	}
}

func TestParseHandler_Upstream(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/parse",
		strings.NewReader(`{"url":"https://v.douyin.com/x/"}`))
	req.Header.Set("Content-Type", "application/json")

	NewHandler(stubExtract("", douyin.ErrUpstream)).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", rec.Code)
	}
	var got map[string]any
	mustDecode(t, rec.Body, &got)
	if int(got["code"].(float64)) != 2001 {
		t.Fatalf("want code 2001, got %v", got["code"])
	}
}

func TestParseHandler_NoPlayURL(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/parse",
		strings.NewReader(`{"url":"https://v.douyin.com/x/"}`))
	req.Header.Set("Content-Type", "application/json")

	NewHandler(stubExtract("", douyin.ErrNoPlayURL)).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", rec.Code)
	}
	var got map[string]any
	mustDecode(t, rec.Body, &got)
	if int(got["code"].(float64)) != 2002 {
		t.Fatalf("want code 2002, got %v", got["code"])
	}
}

func TestParseHandler_Success(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/parse",
		strings.NewReader(`{"url":"https://v.douyin.com/x/"}`))
	req.Header.Set("Content-Type", "application/json")

	NewHandler(stubExtract("https://cdn.example.com/play/v.mp4", nil)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var got map[string]any
	mustDecode(t, rec.Body, &got)
	if int(got["code"].(float64)) != 0 {
		t.Fatalf("want code 0, got %v", got["code"])
	}
	if got["play_url"] != "https://cdn.example.com/play/v.mp4" {
		t.Fatalf("unexpected play_url: %v", got["play_url"])
	}
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	NewHandler(stubExtract("", nil)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ok") {
		t.Fatalf("want body to contain ok, got %q", rec.Body.String())
	}
}

// helpers

func stubExtract(url string, err error) ExtractFunc {
	return func(input string) (string, error) {
		if err != nil {
			return "", err
		}
		return url, nil
	}
}

func mustDecode(t *testing.T, r io.Reader, v any) {
	t.Helper()
	if err := json.NewDecoder(r).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

```

- [ ] **Step 2: Verify test fails**

```bash
go test ./internal/httpapi/
```

Expected: compile error `undefined: NewHandler` / `undefined: ExtractFunc`.

- [ ] **Step 3: Implement**

Create `internal/httpapi/handler.go`:

```go
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/procmeans/sharelink/internal/douyin"
)

// ExtractFunc is the dependency injected into the handler so tests can stub
// the extraction logic without touching the real implementation.
type ExtractFunc func(input string) (string, error)

type parseRequest struct {
	URL string `json:"url"`
}

type successResponse struct {
	Code    int    `json:"code"`
	PlayURL string `json:"play_url"`
}

type errorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewHandler returns the top-level http.Handler for the service.
func NewHandler(extract ExtractFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/api/parse", parseHandler(extract))
	return mux
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

func parseHandler(extract ExtractFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, 1001, "method not allowed")
			return
		}

		var req parseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, 1001, "invalid json body")
			return
		}
		if strings.TrimSpace(req.URL) == "" {
			writeError(w, http.StatusBadRequest, 1001, "url is required")
			return
		}

		playURL, err := extract(req.URL)
		if err != nil {
			status, code := mapError(err)
			writeError(w, status, code, err.Error())
			logRequest(req.URL, "", "error", code, time.Since(start))
			return
		}

		writeJSON(w, http.StatusOK, successResponse{Code: 0, PlayURL: playURL})
		logRequest(req.URL, playURL, "ok", 0, time.Since(start))
	}
}

func mapError(err error) (int, int) {
	switch {
	case errors.Is(err, douyin.ErrNoShortURL):
		return http.StatusUnprocessableEntity, 1002
	case errors.Is(err, douyin.ErrInvalidRedirect):
		return http.StatusUnprocessableEntity, 1003
	case errors.Is(err, douyin.ErrUpstream):
		return http.StatusBadGateway, 2001
	case errors.Is(err, douyin.ErrNoPlayURL):
		return http.StatusBadGateway, 2002
	default:
		return http.StatusInternalServerError, 1000
	}
}

func writeError(w http.ResponseWriter, status, code int, msg string) {
	writeJSON(w, status, errorResponse{Code: code, Message: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func logRequest(input, playURL, result string, code int, dur time.Duration) {
	in := input
	if len(in) > 100 {
		in = in[:100]
	}
	slog.Info("parse",
		"input", in,
		"play_url", playURL,
		"result", result,
		"code", code,
		"duration_ms", dur.Milliseconds(),
	)
}
```

- [ ] **Step 4: Verify test passes**

```bash
go test ./internal/httpapi/ -v
```

Expected: PASS for all subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/handler.go internal/httpapi/handler_test.go
git commit -m "feat(httpapi): add /api/parse handler with error mapping"
```

---

## Task 11: Server entrypoint

**Files:**
- Create: `cmd/server/main.go` (replacing the placeholder `doc.go` from Task 1)
- Modify: delete `cmd/server/doc.go`

- [ ] **Step 1: Remove placeholder doc file**

```bash
rm cmd/server/doc.go
```

- [ ] **Step 2: Write `main.go`**

```go
// Package main is the HTTP server entrypoint for the Douyin direct-link service.
package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/procmeans/sharelink/internal/douyin"
	"github.com/procmeans/sharelink/internal/httpapi"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	handler := httpapi.NewHandler(douyin.Extract)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	slog.Info("server starting", "addr", srv.Addr)
	err := srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Build the binary**

```bash
go build -o server ./cmd/server
```

Expected: produces `./server` in repo root, no errors.

- [ ] **Step 4: Smoke-test boot (start, hit /healthz, kill)**

In one terminal (or background):

```bash
PORT=18080 ./server &
SERVER_PID=$!
sleep 1
curl -fsS http://localhost:18080/healthz
echo
kill $SERVER_PID
wait $SERVER_PID 2>/dev/null
```

Expected: `ok` is printed; the server logs the start line and exits cleanly.

- [ ] **Step 5: Commit**

```bash
git add cmd/server/main.go
git rm --cached cmd/server/doc.go 2>/dev/null || true
git commit -m "feat(cmd/server): wire HTTP server with /api/parse and /healthz"
```

---

## Task 12: Full check + README

**Files:**
- Create: `README.md`

- [ ] **Step 1: Run full test suite**

```bash
go test ./...
```

Expected: all packages PASS, no skips.

- [ ] **Step 2: Run `go vet`**

```bash
go vet ./...
```

Expected: no output.

- [ ] **Step 3: Write README**

Create `README.md`:

````markdown
# sharelink

A small Go HTTP service that takes a Douyin (抖音) share text or short link and returns the watermark-free mp4 CDN URL.

## Run

```bash
go build -o server ./cmd/server
PORT=8080 ./server
```

## API

### `POST /api/parse`

Request:

```json
{ "url": "5.99 fyu:/ 复制打开抖音... https://v.douyin.com/iABCxyz/ ..." }
```

Success response:

```json
{ "code": 0, "play_url": "https://v3-web.douyinvod.com/.../play/...mp4" }
```

Error response:

```json
{ "code": 2001, "message": "douyin: upstream request failed: ..." }
```

### Error codes

| code | HTTP | meaning |
|------|------|---------|
| 0    | 200  | success |
| 1001 | 400  | bad request body |
| 1002 | 422  | no v.douyin.com short URL in input |
| 1003 | 422  | redirect target had no video id |
| 2001 | 502  | upstream request failed |
| 2002 | 502  | RENDER_DATA missing or play URL empty |

### `GET /healthz`

Returns `200 ok`. For health checks.

## Test

```bash
go test ./...
```

All tests are offline; no real Douyin requests.
````

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add README"
```

- [ ] **Step 5: Final acceptance check**

Run, in order:

```bash
go test ./...
go vet ./...
go build -o server ./cmd/server
```

Expected: all three succeed silently. The acceptance criteria from the spec (offline tests pass, no vet warnings, server builds) are met.

---

## Out of Scope (Reminder)

Per spec, these are intentionally NOT in this plan:
- Image-post / audio extraction
- Server-side proxy download (`/download` returning the file body)
- Caching, retry, multi-quality selection
- API key auth, IP rate limiting
- Real Douyin integration tests (manual `cmd/probe` left as a follow-up if desired)
