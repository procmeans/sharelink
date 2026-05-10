# ttwid Cookie + Memory Cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce Douyin rate-limit risk and unnecessary upstream calls by adding (a) a ttwid cookie pulled from douyin.com at startup and refreshed on share-parse failure, and (b) an in-process LRU+TTL cache keyed by `video_id`.

**Architecture:** New `internal/cache` package (generic string→string LRU+TTL). New `internal/douyin/ttwid.go` (TTWidManager — fetches and stores `ttwid` cookie). New `internal/douyin/service.go` (`Service` struct holding ttwid + cache; replaces package-level `Extract`/`ExtractWithConfig`). `ResolveVideoID` and `fetchPlayURLFromBase` gain a `cookie string` parameter. `cmd/server/main.go` wires it all together.

**Tech Stack:** Go (standard library only — `sync`, `container/list`, `time`, `net/http`, etc).

---

## File Structure

| Path | New / Modified | Purpose |
|------|----------------|---------|
| `internal/cache/doc.go` | New | Package marker + one-line doc |
| `internal/cache/cache.go` | New | `Cache` struct: LRU+TTL string→string store |
| `internal/cache/cache_test.go` | New | Tests for TTL, LRU, concurrency, edge cases |
| `internal/douyin/ttwid.go` | New | `TTWidManager` — Refresh / Cookie |
| `internal/douyin/ttwid_test.go` | New | httptest-based tests for ttwid extraction |
| `internal/douyin/service.go` | New | `Service` struct — orchestrates resolve→cache→fetch→retry→strip |
| `internal/douyin/service_test.go` | New | End-to-end tests (replaces extract_test.go) |
| `internal/douyin/extract.go` | Deleted | Replaced by service.go |
| `internal/douyin/extract_test.go` | Deleted | Replaced by service_test.go |
| `internal/douyin/resolve.go` | Modified | Add `cookie string` parameter |
| `internal/douyin/resolve_test.go` | Modified | Pass `""` cookie in existing assertions; add 1 cookie-propagation test |
| `internal/douyin/share.go` | Modified | Add `cookie string` parameter; remove unused `FetchPlayURL` |
| `internal/douyin/share_test.go` | Modified | Pass `""` cookie; add 1 cookie-propagation test |
| `cmd/server/main.go` | Modified | Build TTWidManager, Cache, Service; wire to handler |
| `internal/httpapi/handler_test.go` | (verify only) | Stub still matches `func(string)(douyin.Result,error)` — no change needed |

---

## Task 1: Cache package — directory + doc

**Files:**
- Create: `internal/cache/doc.go`

- [ ] **Step 1: Create the package marker**

Create `internal/cache/doc.go`:

```go
// Package cache provides a small in-process LRU+TTL string-to-string cache.
package cache
```

- [ ] **Step 2: Build**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/cache/doc.go
git commit -m "chore(cache): add package marker"
```

---

## Task 2: Cache — minimal Get/Set with TTL (no LRU yet)

**Files:**
- Create: `internal/cache/cache.go`
- Create: `internal/cache/cache_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cache/cache_test.go`:

```go
package cache

import (
	"testing"
	"time"
)

func TestCache_GetMiss(t *testing.T) {
	c := New(10, time.Minute)
	if _, ok := c.Get("nope"); ok {
		t.Fatal("expected miss")
	}
}

func TestCache_SetAndGet(t *testing.T) {
	c := New(10, time.Minute)
	c.Set("k", "v")
	got, ok := c.Get("k")
	if !ok || got != "v" {
		t.Fatalf("want v/true, got %q/%v", got, ok)
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	c := New(10, 20*time.Millisecond)
	c.Set("k", "v")
	time.Sleep(40 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatal("expected expired miss")
	}
}

func TestCache_OverwriteSameKey(t *testing.T) {
	c := New(10, time.Minute)
	c.Set("k", "v1")
	c.Set("k", "v2")
	got, _ := c.Get("k")
	if got != "v2" {
		t.Fatalf("want v2, got %q", got)
	}
	if c.Len() != 1 {
		t.Fatalf("want len 1, got %d", c.Len())
	}
}
```

- [ ] **Step 2: Verify it fails**

```bash
go test ./internal/cache/
```

Expected: compile error `undefined: New` etc.

- [ ] **Step 3: Implement (TTL only, simple map for now)**

Create `internal/cache/cache.go`:

```go
package cache

import (
	"sync"
	"time"
)

type entry struct {
	value     string
	expiresAt time.Time
}

// Cache is an in-process string-to-string cache with per-entry TTL and a
// max-entries cap (LRU eviction added in a later task).
type Cache struct {
	mu          sync.Mutex
	maxEntries  int
	ttl         time.Duration
	items       map[string]*entry
}

func New(maxEntries int, ttl time.Duration) *Cache {
	return &Cache{
		maxEntries: maxEntries,
		ttl:        ttl,
		items:      make(map[string]*entry, maxEntries),
	}
}

func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		return "", false
	}
	if time.Now().After(e.expiresAt) {
		delete(c.items, key)
		return "", false
	}
	return e.value, true
}

func (c *Cache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = &entry{value: value, expiresAt: time.Now().Add(c.ttl)}
}

func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}
```

- [ ] **Step 4: Verify all 4 tests pass**

```bash
go test ./internal/cache/ -v
```

Expected: PASS for all 4 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/cache/cache.go internal/cache/cache_test.go
git commit -m "feat(cache): basic Get/Set with TTL"
```

---

## Task 3: Cache — LRU eviction

**Files:**
- Modify: `internal/cache/cache.go`
- Modify: `internal/cache/cache_test.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/cache/cache_test.go`:

```go
func TestCache_LRUEviction(t *testing.T) {
	c := New(3, time.Minute)
	c.Set("a", "1")
	c.Set("b", "2")
	c.Set("c", "3")
	// "a" is the oldest. Touching "a" via Get should make "b" the oldest.
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a should be present")
	}
	c.Set("d", "4") // evicts the LRU
	if _, ok := c.Get("b"); ok {
		t.Fatal("b should have been evicted (was LRU)")
	}
	for _, k := range []string{"a", "c", "d"} {
		if _, ok := c.Get(k); !ok {
			t.Fatalf("%s should still be present", k)
		}
	}
	if c.Len() != 3 {
		t.Fatalf("want len 3, got %d", c.Len())
	}
}

func TestCache_OverwritePromotes(t *testing.T) {
	c := New(2, time.Minute)
	c.Set("a", "1")
	c.Set("b", "2")
	c.Set("a", "1b") // should promote a, not evict
	c.Set("c", "3")  // should evict b
	if _, ok := c.Get("b"); ok {
		t.Fatal("b should have been evicted")
	}
	if got, _ := c.Get("a"); got != "1b" {
		t.Fatalf("want a=1b, got %q", got)
	}
}

func TestCache_ZeroCapacity(t *testing.T) {
	c := New(0, time.Minute)
	c.Set("a", "1")
	if _, ok := c.Get("a"); ok {
		t.Fatal("zero-capacity cache must hold nothing")
	}
}
```

- [ ] **Step 2: Verify they fail**

```bash
go test ./internal/cache/ -run 'TestCache_LRU|TestCache_Overwrite|TestCache_Zero' -v
```

Expected: TestCache_LRUEviction fails (no eviction logic yet); TestCache_OverwritePromotes likely fails; TestCache_ZeroCapacity fails.

- [ ] **Step 3: Rewrite `cache.go` with LRU**

Replace `internal/cache/cache.go` entirely:

```go
package cache

import (
	"container/list"
	"sync"
	"time"
)

type entry struct {
	key       string
	value     string
	expiresAt time.Time
}

// Cache is an in-process string-to-string cache with per-entry TTL and
// LRU eviction when maxEntries is reached.
type Cache struct {
	mu         sync.Mutex
	maxEntries int
	ttl        time.Duration
	items      map[string]*list.Element // value type: *entry
	order      *list.List               // front = MRU, back = LRU
}

func New(maxEntries int, ttl time.Duration) *Cache {
	return &Cache{
		maxEntries: maxEntries,
		ttl:        ttl,
		items:      make(map[string]*list.Element),
		order:      list.New(),
	}
}

func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	e := el.Value.(*entry)
	if time.Now().After(e.expiresAt) {
		c.order.Remove(el)
		delete(c.items, key)
		return "", false
	}
	c.order.MoveToFront(el)
	return e.value, true
}

func (c *Cache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maxEntries <= 0 {
		return
	}
	if el, ok := c.items[key]; ok {
		e := el.Value.(*entry)
		e.value = value
		e.expiresAt = time.Now().Add(c.ttl)
		c.order.MoveToFront(el)
		return
	}
	e := &entry{key: key, value: value, expiresAt: time.Now().Add(c.ttl)}
	el := c.order.PushFront(e)
	c.items[key] = el
	if c.order.Len() > c.maxEntries {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*entry).key)
		}
	}
}

func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}
```

- [ ] **Step 4: Verify all cache tests pass**

```bash
go test ./internal/cache/ -v
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cache/cache.go internal/cache/cache_test.go
git commit -m "feat(cache): add LRU eviction"
```

---

## Task 4: Cache — race test

**Files:**
- Modify: `internal/cache/cache_test.go`

- [ ] **Step 1: Add concurrency test**

Append to `internal/cache/cache_test.go`:

```go
func TestCache_ConcurrentReadsAndWrites(t *testing.T) {
	c := New(100, time.Minute)
	const goroutines = 16
	const iterations = 1000

	done := make(chan struct{}, goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < iterations; i++ {
				key := string(rune('a' + (i+id)%26))
				if i%2 == 0 {
					c.Set(key, key)
				} else {
					_, _ = c.Get(key)
				}
			}
		}(g)
	}
	for g := 0; g < goroutines; g++ {
		<-done
	}
}
```

- [ ] **Step 2: Run with race detector**

```bash
go test ./internal/cache/ -race -count=1
```

Expected: PASS, no DATA RACE warnings.

- [ ] **Step 3: Commit**

```bash
git add internal/cache/cache_test.go
git commit -m "test(cache): add concurrency test"
```

---

## Task 5: TTWidManager

**Files:**
- Create: `internal/douyin/ttwid.go`
- Create: `internal/douyin/ttwid_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/douyin/ttwid_test.go`:

```go
package douyin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/procmeans/sharelink/internal/httpclient"
)

func TestTTWid_RefreshSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "ttwid", Value: "abc123"})
		http.SetCookie(w, &http.Cookie{Name: "other", Value: "ignored"})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := newTTWidManagerWithURL(httpclient.NewFollowing(), srv.URL)
	if err := m.Refresh(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if m.Cookie() != "abc123" {
		t.Fatalf("want abc123, got %q", m.Cookie())
	}
}

func TestTTWid_RefreshNoCookie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := newTTWidManagerWithURL(httpclient.NewFollowing(), srv.URL)
	err := m.Refresh()
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("want ErrUpstream, got %v", err)
	}
	if m.Cookie() != "" {
		t.Fatalf("Cookie() should remain empty, got %q", m.Cookie())
	}
}

func TestTTWid_RefreshHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := newTTWidManagerWithURL(httpclient.NewFollowing(), srv.URL)
	err := m.Refresh()
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("want ErrUpstream, got %v", err)
	}
}

func TestTTWid_CookieEmptyBeforeRefresh(t *testing.T) {
	m := newTTWidManagerWithURL(httpclient.NewFollowing(), "http://unused")
	if m.Cookie() != "" {
		t.Fatalf("want empty cookie pre-refresh, got %q", m.Cookie())
	}
}

func TestTTWid_RefreshFailureDoesNotClearExistingCookie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "ttwid", Value: "good"})
	}))
	m := newTTWidManagerWithURL(httpclient.NewFollowing(), srv.URL)
	if err := m.Refresh(); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	srv.Close()

	// Now point the manager at a dead URL and Refresh again — must not
	// clobber the previously-acquired good value.
	m = m.withURLForTest("http://127.0.0.1:1") // unreachable
	_ = m.Refresh()
	if m.Cookie() != "good" {
		t.Fatalf("expected previous cookie preserved, got %q", m.Cookie())
	}
}
```

- [ ] **Step 2: Verify it fails**

```bash
go test ./internal/douyin/ -run TestTTWid
```

Expected: compile error `undefined: newTTWidManagerWithURL` etc.

- [ ] **Step 3: Implement**

Create `internal/douyin/ttwid.go`:

```go
package douyin

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/procmeans/sharelink/internal/httpclient"
)

const ttwidSourceURL = "https://www.douyin.com/"

// TTWidManager fetches and stores the douyin "ttwid" cookie.
type TTWidManager struct {
	client *http.Client
	url    string

	mu    sync.RWMutex
	value string
}

// NewTTWidManager constructs a manager that fetches ttwid from douyin.com.
func NewTTWidManager(client *http.Client) *TTWidManager {
	return newTTWidManagerWithURL(client, ttwidSourceURL)
}

// newTTWidManagerWithURL is the test seam.
func newTTWidManagerWithURL(client *http.Client, url string) *TTWidManager {
	return &TTWidManager{client: client, url: url}
}

// withURLForTest returns the same manager with the source URL replaced.
// Used only by tests that need to point an existing manager at a different host.
func (m *TTWidManager) withURLForTest(url string) *TTWidManager {
	m.mu.Lock()
	m.url = url
	m.mu.Unlock()
	return m
}

// Cookie returns the current ttwid value, or "" if none has been acquired.
func (m *TTWidManager) Cookie() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.value
}

// Refresh fetches a fresh ttwid from the source URL. The HTTP request is made
// without holding the write lock, so concurrent Cookie() reads are not blocked.
// On failure the previously-stored value is preserved.
func (m *TTWidManager) Refresh() error {
	m.mu.RLock()
	url := m.url
	m.mu.RUnlock()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("%w: build ttwid request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", httpclient.MobileUA)

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: do ttwid request: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: ttwid status %d", ErrUpstream, resp.StatusCode)
	}

	var found string
	for _, ck := range resp.Cookies() {
		if ck.Name == "ttwid" {
			found = ck.Value
			break
		}
	}
	if found == "" {
		return fmt.Errorf("%w: ttwid cookie not in Set-Cookie", ErrUpstream)
	}

	m.mu.Lock()
	m.value = found
	m.mu.Unlock()
	return nil
}
```

- [ ] **Step 4: Verify tests pass**

```bash
go test ./internal/douyin/ -run TestTTWid -v
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/douyin/ttwid.go internal/douyin/ttwid_test.go
git commit -m "feat(douyin): add TTWidManager"
```

---

## Task 6: ResolveVideoID — accept cookie parameter

**Files:**
- Modify: `internal/douyin/resolve.go`
- Modify: `internal/douyin/resolve_test.go`

- [ ] **Step 1: Update existing tests to use the new signature**

Replace the four existing subtests inside `TestResolveVideoID` so each call site becomes `ResolveVideoID(httpclient.NewNoRedirect(), "", srv.URL)` — i.e. add `""` as the new second argument. Keep the assertions identical.

Also append a new test that asserts the cookie ends up on the wire:

```go
func TestResolveVideoID_CookieHeaderSet(t *testing.T) {
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Location", "https://www.iesdouyin.com/share/video/7000000000000000000/")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	if _, err := ResolveVideoID(httpclient.NewNoRedirect(), "abc123", srv.URL); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotCookie != "ttwid=abc123" {
		t.Fatalf("want Cookie header 'ttwid=abc123', got %q", gotCookie)
	}
}

func TestResolveVideoID_NoCookieHeaderWhenEmpty(t *testing.T) {
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Location", "https://www.iesdouyin.com/share/video/7000000000000000000/")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	if _, err := ResolveVideoID(httpclient.NewNoRedirect(), "", srv.URL); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotCookie != "" {
		t.Fatalf("expected no Cookie header, got %q", gotCookie)
	}
}
```

- [ ] **Step 2: Verify the existing tests now fail at compile time**

```bash
go test ./internal/douyin/ -run TestResolveVideoID
```

Expected: compile error: too few arguments in call to ResolveVideoID.

- [ ] **Step 3: Modify implementation**

Edit `internal/douyin/resolve.go`. Change the function signature and add the cookie-setting logic:

```go
func ResolveVideoID(client *http.Client, cookie, shortURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, shortURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", httpclient.MobileUA)
	if cookie != "" {
		req.Header.Set("Cookie", "ttwid="+cookie)
	}

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

- [ ] **Step 4: Verify tests pass**

```bash
go test ./internal/douyin/ -run TestResolveVideoID -v
```

Expected: all 6 subtests PASS (4 existing + 2 new).

- [ ] **Step 5: Commit**

```bash
git add internal/douyin/resolve.go internal/douyin/resolve_test.go
git commit -m "feat(douyin): pass ttwid cookie through ResolveVideoID"
```

---

## Task 7: fetchPlayURLFromBase — accept cookie parameter

**Files:**
- Modify: `internal/douyin/share.go`
- Modify: `internal/douyin/share_test.go`

- [ ] **Step 1: Update existing tests**

Edit each call to `fetchPlayURLFromBase(httpclient.NewFollowing(), srv.URL, "...")` and add `""` as the new second argument:
`fetchPlayURLFromBase(httpclient.NewFollowing(), "", srv.URL, "...")`. Keep all assertions identical.

Append two new tests:

```go
func TestFetchPlayURL_CookieHeaderSet(t *testing.T) {
	body, err := os.ReadFile("testdata/sample_share.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, string(body))
	}))
	defer srv.Close()

	if _, err := fetchPlayURLFromBase(httpclient.NewFollowing(), "abc123", srv.URL, "7234567890123456789"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotCookie != "ttwid=abc123" {
		t.Fatalf("want Cookie header 'ttwid=abc123', got %q", gotCookie)
	}
}

func TestFetchPlayURL_NoCookieHeaderWhenEmpty(t *testing.T) {
	body, err := os.ReadFile("testdata/sample_share.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, string(body))
	}))
	defer srv.Close()

	if _, err := fetchPlayURLFromBase(httpclient.NewFollowing(), "", srv.URL, "7234567890123456789"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotCookie != "" {
		t.Fatalf("expected no Cookie header, got %q", gotCookie)
	}
}
```

- [ ] **Step 2: Verify the existing tests now fail at compile time**

```bash
go test ./internal/douyin/ -run TestFetchPlayURL
```

Expected: compile error: too few arguments in call to fetchPlayURLFromBase.

- [ ] **Step 3: Modify implementation**

Edit `internal/douyin/share.go`. Change the function signature, remove the now-unused `FetchPlayURL` wrapper, and add cookie logic:

```go
// remove the old `FetchPlayURL` function entirely

func fetchPlayURLFromBase(client *http.Client, cookie, baseURL, videoID string) (string, error) {
	target := fmt.Sprintf("%s/share/video/%s/", baseURL, videoID)

	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", httpclient.MobileUA)
	if cookie != "" {
		req.Header.Set("Cookie", "ttwid="+cookie)
	}

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

	idx := bytes.Index(body, routerDataPrefix)
	if idx < 0 {
		return "", fmt.Errorf("%w: _ROUTER_DATA marker not found", ErrNoPlayURL)
	}
	jsonStart := idx + len(routerDataPrefix)

	var rd routerData
	dec := json.NewDecoder(bytes.NewReader(body[jsonStart:]))
	if err := dec.Decode(&rd); err != nil {
		return "", fmt.Errorf("%w: decode _ROUTER_DATA: %v", ErrNoPlayURL, err)
	}

	items := rd.LoaderData.Page.VideoInfoRes.ItemList
	if len(items) == 0 {
		return "", fmt.Errorf("%w: empty item_list", ErrNoPlayURL)
	}
	urls := items[0].Video.PlayAddr.URLList
	if len(urls) == 0 || urls[0] == "" {
		return "", fmt.Errorf("%w: empty url_list", ErrNoPlayURL)
	}
	return urls[0], nil
}
```

- [ ] **Step 4: Verify tests pass**

```bash
go test ./internal/douyin/ -run TestFetchPlayURL -v
```

Expected: all 6 subtests PASS (4 existing + 2 new).

- [ ] **Step 5: Commit**

```bash
git add internal/douyin/share.go internal/douyin/share_test.go
git commit -m "feat(douyin): pass ttwid cookie through fetchPlayURLFromBase"
```

---

## Task 8: Service — replace package-level Extract

**Files:**
- Create: `internal/douyin/service.go`
- Create: `internal/douyin/service_test.go`
- Delete: `internal/douyin/extract.go`
- Delete: `internal/douyin/extract_test.go`
- Modify: `cmd/server/main.go` (just enough to keep compilation green)

- [ ] **Step 1: Write the failing test**

Create `internal/douyin/service_test.go`:

```go
package douyin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/procmeans/sharelink/internal/cache"
	"github.com/procmeans/sharelink/internal/httpclient"
)

// fakeDouyin spins up an httptest server that simulates BOTH the short-URL
// 302 and the iesdouyin.com share page in one mux. The behavior of the share
// route is controlled by the supplied bodies slice — each request consumes the
// next body. After all bodies are consumed the test fails the request hard.
func fakeDouyin(t *testing.T, videoID string, shareBodies [][]byte) (
	srv *httptest.Server,
	shortURL string,
	shareCalls *int32,
) {
	t.Helper()
	mux := http.NewServeMux()
	srv = httptest.NewUnstartedServer(mux)

	var calls int32
	shareCalls = &calls

	mux.HandleFunc("/short/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", srv.URL+"/share/video/"+videoID+"/")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/share/video/", func(w http.ResponseWriter, r *http.Request) {
		i := int(atomic.AddInt32(&calls, 1)) - 1
		if i >= len(shareBodies) {
			t.Errorf("unexpected extra share request #%d", i+1)
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(shareBodies[i])
	})

	srv.Start()
	return srv, srv.URL + "/short/abc", shareCalls
}

func mustReadFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/sample_share.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

// stubTTWid is a TTWidManager pointed at a dead URL but with an arbitrary
// pre-seeded value, so Cookie() returns whatever we want and Refresh() fails
// (or succeeds via a separate httptest server if one is provided).
func stubTTWid(value string) *TTWidManager {
	m := newTTWidManagerWithURL(httpclient.NewFollowing(), "http://127.0.0.1:1")
	m.mu.Lock()
	m.value = value
	m.mu.Unlock()
	return m
}

func TestService_CacheMiss_ThenStores(t *testing.T) {
	fixture := mustReadFixture(t)
	srv, shortURL, shareCalls := fakeDouyin(t, "7234567890123456789", [][]byte{fixture})
	defer srv.Close()

	c := cache.New(10, time.Minute)
	svc := NewService(stubTTWid(""), c)

	res, err := svc.ExtractWithConfig(Config{ShortURL: shortURL, ShareBaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "https://aweme.snssdk.com/aweme/v1/play/?video_id=v0200fg10000abc&line=0"
	if res.PlayURL != want {
		t.Fatalf("want %q, got %q", want, res.PlayURL)
	}
	if res.VideoID != "7234567890123456789" {
		t.Fatalf("want video id, got %q", res.VideoID)
	}
	if got, ok := c.Get("7234567890123456789"); !ok || got != want {
		t.Fatalf("cache should hold play_url, got %q/%v", got, ok)
	}
	if *shareCalls != 1 {
		t.Fatalf("want 1 share call, got %d", *shareCalls)
	}
}

func TestService_CacheHit_DoesNotCallShare(t *testing.T) {
	srv, shortURL, shareCalls := fakeDouyin(t, "7234567890123456789", [][]byte{})
	defer srv.Close()

	c := cache.New(10, time.Minute)
	c.Set("7234567890123456789", "https://cached.example/x.mp4")

	svc := NewService(stubTTWid(""), c)
	res, err := svc.ExtractWithConfig(Config{ShortURL: shortURL, ShareBaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.PlayURL != "https://cached.example/x.mp4" {
		t.Fatalf("want cached play_url, got %q", res.PlayURL)
	}
	if *shareCalls != 0 {
		t.Fatalf("share should not have been called, but was %d times", *shareCalls)
	}
}

func TestService_NoPlayURL_RefreshTTWid_ThenSucceeds(t *testing.T) {
	fixture := mustReadFixture(t)
	emptyShare := []byte(`<html><body>nothing</body></html>`) // triggers ErrNoPlayURL
	srv, shortURL, shareCalls := fakeDouyin(t, "7234567890123456789", [][]byte{emptyShare, fixture})
	defer srv.Close()

	// ttwid manager backed by its own server so Refresh() can succeed
	ttwidSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "ttwid", Value: "freshval"})
	}))
	defer ttwidSrv.Close()
	ttwid := newTTWidManagerWithURL(httpclient.NewFollowing(), ttwidSrv.URL)

	svc := NewService(ttwid, cache.New(10, time.Minute))
	res, err := svc.ExtractWithConfig(Config{ShortURL: shortURL, ShareBaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.PlayURL == "" {
		t.Fatal("want non-empty play_url after retry")
	}
	if *shareCalls != 2 {
		t.Fatalf("want 2 share calls (1 initial + 1 retry), got %d", *shareCalls)
	}
	if ttwid.Cookie() != "freshval" {
		t.Fatalf("want ttwid refreshed to freshval, got %q", ttwid.Cookie())
	}
}

func TestService_NoPlayURL_RetryAlsoFails(t *testing.T) {
	emptyShare := []byte(`<html><body>nothing</body></html>`)
	srv, shortURL, shareCalls := fakeDouyin(t, "7234567890123456789", [][]byte{emptyShare, emptyShare})
	defer srv.Close()

	ttwidSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "ttwid", Value: "x"})
	}))
	defer ttwidSrv.Close()
	ttwid := newTTWidManagerWithURL(httpclient.NewFollowing(), ttwidSrv.URL)

	svc := NewService(ttwid, cache.New(10, time.Minute))
	_, err := svc.ExtractWithConfig(Config{ShortURL: shortURL, ShareBaseURL: srv.URL})
	if !errors.Is(err, ErrNoPlayURL) {
		t.Fatalf("want ErrNoPlayURL, got %v", err)
	}
	if *shareCalls != 2 {
		t.Fatalf("want exactly 2 share calls (initial + 1 retry), got %d", *shareCalls)
	}
}

func TestService_Extract_NoShortURL(t *testing.T) {
	svc := NewService(stubTTWid(""), cache.New(10, time.Minute))
	_, err := svc.Extract("just plain text")
	if !errors.Is(err, ErrNoShortURL) {
		t.Fatalf("want ErrNoShortURL, got %v", err)
	}
}
```

- [ ] **Step 2: Verify it fails (along with the now-broken main.go)**

```bash
go test ./internal/douyin/ -run TestService
```

Expected: compile error `undefined: NewService` etc.

- [ ] **Step 3: Implement Service**

Create `internal/douyin/service.go`:

```go
package douyin

import (
	"errors"

	"github.com/procmeans/sharelink/internal/cache"
	"github.com/procmeans/sharelink/internal/httpclient"
)

// Result is what Extract returns: the watermark-free play URL plus the
// numeric video id (useful for logging/correlation).
type Result struct {
	PlayURL string
	VideoID string
}

// Config lets tests inject a fake short URL and share base.
type Config struct {
	ShortURL     string
	ShareBaseURL string
}

// Service orchestrates the extraction pipeline with ttwid cookies and a
// video_id-keyed cache.
type Service struct {
	ttwid *TTWidManager
	cache *cache.Cache
}

func NewService(ttwid *TTWidManager, c *cache.Cache) *Service {
	return &Service{ttwid: ttwid, cache: c}
}

// Extract is the high-level entry point: take user-pasted share text or a
// short URL and return a watermark-free mp4 URL plus the video id.
func (s *Service) Extract(input string) (Result, error) {
	short, err := ExtractShortURL(input)
	if err != nil {
		return Result{}, err
	}
	return s.ExtractWithConfig(Config{ShortURL: short})
}

// ExtractWithConfig runs resolve → cache → fetch → optional refresh+retry → strip.
func (s *Service) ExtractWithConfig(cfg Config) (Result, error) {
	if cfg.ShareBaseURL == "" {
		cfg.ShareBaseURL = shareBaseURL
	}

	videoID, err := ResolveVideoID(httpclient.NewNoRedirect(), s.ttwid.Cookie(), cfg.ShortURL)
	if err != nil {
		return Result{}, err
	}

	if cached, ok := s.cache.Get(videoID); ok {
		return Result{PlayURL: cached, VideoID: videoID}, nil
	}

	shareClient := httpclient.NewFollowing()
	playWM, err := fetchPlayURLFromBase(shareClient, s.ttwid.Cookie(), cfg.ShareBaseURL, videoID)
	if errors.Is(err, ErrNoPlayURL) {
		// Refresh ttwid and retry the share step exactly once.
		if rerr := s.ttwid.Refresh(); rerr != nil {
			return Result{VideoID: videoID}, err
		}
		playWM, err = fetchPlayURLFromBase(shareClient, s.ttwid.Cookie(), cfg.ShareBaseURL, videoID)
	}
	if err != nil {
		return Result{VideoID: videoID}, err
	}

	playURL := StripWatermark(playWM)
	s.cache.Set(videoID, playURL)
	return Result{PlayURL: playURL, VideoID: videoID}, nil
}
```

- [ ] **Step 4: Delete old extract files**

```bash
rm internal/douyin/extract.go internal/douyin/extract_test.go
```

- [ ] **Step 5: Patch main.go to compile (real wiring comes in Task 9)**

For now, edit `cmd/server/main.go` so it compiles. The two existing references to `douyin.Extract` need to become `svc.Extract` of a Service built with placeholder dependencies. Replace the body of `main()` with:

```go
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	// Temporary placeholder wiring; real ttwid + cache assembly is Task 9.
	ttwid := douyin.NewTTWidManager(httpclient.NewFollowing())
	c := cache.New(1000, 30*time.Minute)
	svc := douyin.NewService(ttwid, c)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	handler := httpapi.NewHandler(svc.Extract)

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

Update the import block to add `"github.com/procmeans/sharelink/internal/cache"` and `"github.com/procmeans/sharelink/internal/httpclient"`.

- [ ] **Step 6: Verify everything builds and all package tests pass**

```bash
go build ./...
go test ./... -count=1
```

Expected: clean build; all tests including the 5 new TestService_* pass.

- [ ] **Step 7: Commit**

```bash
git add internal/douyin/service.go internal/douyin/service_test.go cmd/server/main.go
git rm internal/douyin/extract.go internal/douyin/extract_test.go
git commit -m "feat(douyin): replace package-level Extract with Service (ttwid+cache)"
```

---

## Task 9: Wire startup ttwid Refresh + JSON logging fields

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `internal/httpapi/handler.go` (add cache_hit + ttwid_refreshed fields)

This task adds the production-side wiring the spec demands: blocking ttwid refresh at startup, plus the two new log fields.

- [ ] **Step 1: Update main.go to block-Refresh at startup**

Edit the body of `main()` so it calls `ttwid.Refresh()` after construction and exits non-zero on failure:

```go
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	ttwid := douyin.NewTTWidManager(httpclient.NewFollowing())
	if err := ttwid.Refresh(); err != nil {
		slog.Error("ttwid refresh failed at startup", "err", err.Error())
		os.Exit(1)
	}
	slog.Info("ttwid acquired", "len", len(ttwid.Cookie()))

	c := cache.New(1000, 30*time.Minute)
	svc := douyin.NewService(ttwid, c)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	handler := httpapi.NewHandler(svc.Extract)

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

- [ ] **Step 2: Build and run a smoke check**

```bash
go build -o server ./cmd/server
PORT=18080 ./server > /tmp/sharelink.log 2>&1 &
SERVER_PID=$!
sleep 2
curl -fsS http://localhost:18080/healthz
echo
kill $SERVER_PID
wait $SERVER_PID 2>/dev/null
grep '"ttwid acquired"' /tmp/sharelink.log
```

Expected: `ok` is printed, and the log file contains a JSON line with `"msg":"ttwid acquired"` and an `"len":<positive number>`.

- [ ] **Step 3: Commit the startup wiring**

```bash
git add cmd/server/main.go
git commit -m "feat(cmd/server): block-refresh ttwid at startup"
```

- [ ] **Step 4: (handler) Add cache_hit / ttwid_refreshed log fields — interface change**

The handler currently logs from `parseHandler` after extract returns. The Service knows about cache hits and ttwid refreshes; it must surface them. Extend `Result`:

Edit `internal/douyin/service.go`. Add two fields to `Result`:

```go
type Result struct {
	PlayURL        string
	VideoID        string
	CacheHit       bool
	TTWidRefreshed bool
}
```

Update `ExtractWithConfig` to set them:

- After `if cached, ok := s.cache.Get(videoID); ok { return Result{PlayURL: cached, VideoID: videoID, CacheHit: true}, nil }`.
- Track a local `refreshed bool` that flips to true when the Refresh call is made (regardless of success). Pass it into the returned Result on both success and failure.

Final shape of `ExtractWithConfig`:

```go
func (s *Service) ExtractWithConfig(cfg Config) (Result, error) {
	if cfg.ShareBaseURL == "" {
		cfg.ShareBaseURL = shareBaseURL
	}

	videoID, err := ResolveVideoID(httpclient.NewNoRedirect(), s.ttwid.Cookie(), cfg.ShortURL)
	if err != nil {
		return Result{}, err
	}

	if cached, ok := s.cache.Get(videoID); ok {
		return Result{PlayURL: cached, VideoID: videoID, CacheHit: true}, nil
	}

	shareClient := httpclient.NewFollowing()
	playWM, err := fetchPlayURLFromBase(shareClient, s.ttwid.Cookie(), cfg.ShareBaseURL, videoID)
	refreshed := false
	if errors.Is(err, ErrNoPlayURL) {
		refreshed = true
		if rerr := s.ttwid.Refresh(); rerr != nil {
			return Result{VideoID: videoID, TTWidRefreshed: refreshed}, err
		}
		playWM, err = fetchPlayURLFromBase(shareClient, s.ttwid.Cookie(), cfg.ShareBaseURL, videoID)
	}
	if err != nil {
		return Result{VideoID: videoID, TTWidRefreshed: refreshed}, err
	}

	playURL := StripWatermark(playWM)
	s.cache.Set(videoID, playURL)
	return Result{PlayURL: playURL, VideoID: videoID, TTWidRefreshed: refreshed}, nil
}
```

- [ ] **Step 5: Update existing service_test.go assertions for the new fields**

In `internal/douyin/service_test.go`:

- `TestService_CacheMiss_ThenStores`: add `if res.CacheHit { t.Fatal("expected cache miss") }`.
- `TestService_CacheHit_DoesNotCallShare`: add `if !res.CacheHit { t.Fatal("expected cache hit") }`.
- `TestService_NoPlayURL_RefreshTTWid_ThenSucceeds`: add `if !res.TTWidRefreshed { t.Fatal("expected TTWidRefreshed=true") }`.

- [ ] **Step 6: Update handler to read & log these**

Edit `internal/httpapi/handler.go`. The current `logRequest(req.URL, res.VideoID, res.PlayURL, "ok", 0, ...)` becomes:

```go
logRequest(req.URL, res.VideoID, res.PlayURL, "ok", 0, res.CacheHit, res.TTWidRefreshed, time.Since(start))
```

And in the error branch (after the `if err != nil`):

```go
logRequest(req.URL, res.VideoID, "", "error", code, res.CacheHit, res.TTWidRefreshed, time.Since(start))
```

Update `logRequest` signature:

```go
func logRequest(input, videoID, playURL, result string, code int, cacheHit, ttwidRefreshed bool, dur time.Duration) {
	in := input
	if len(in) > 100 {
		in = in[:100]
	}
	slog.Info("parse",
		"input", in,
		"video_id", videoID,
		"play_url", playURL,
		"result", result,
		"code", code,
		"cache_hit", cacheHit,
		"ttwid_refreshed", ttwidRefreshed,
		"duration_ms", dur.Milliseconds(),
	)
}
```

The `mapError` branches that previously returned just `err.Error()` from the handler before `extract` was even called (bad json / missing url / method not allowed) should pass `false, false` for the new fields.

The full `parseHandler` body, for clarity:

```go
func parseHandler(extract ExtractFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, 1004, "method not allowed")
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

		res, err := extract(req.URL)
		if err != nil {
			status, code := mapError(err)
			writeError(w, status, code, err.Error())
			logRequest(req.URL, res.VideoID, "", "error", code, res.CacheHit, res.TTWidRefreshed, time.Since(start))
			return
		}

		writeJSON(w, http.StatusOK, successResponse{Code: 0, PlayURL: res.PlayURL})
		logRequest(req.URL, res.VideoID, res.PlayURL, "ok", 0, res.CacheHit, res.TTWidRefreshed, time.Since(start))
	}
}
```

- [ ] **Step 7: Verify everything still passes**

```bash
go test ./... -race -count=1
go vet ./...
```

Expected: all PASS, no race warnings, vet clean.

- [ ] **Step 8: Commit**

```bash
git add internal/douyin/service.go internal/douyin/service_test.go internal/httpapi/handler.go
git commit -m "feat(douyin,httpapi): surface cache_hit and ttwid_refreshed in logs"
```

---

## Task 10: Final acceptance — README + real-link smoke test

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update README to mention the new behavior**

Edit `README.md`. Append a new section after the "Test" section:

```markdown
## Operational notes

- On startup the server fetches a `ttwid` cookie from `https://www.douyin.com/`. Failure is fatal — the process exits with status 1.
- Successful parses are cached in-process by `video_id` for 30 minutes (LRU, max 1000 entries). Cache survives only the lifetime of the process.
- If the share-page parser returns "no play URL" the server refreshes ttwid and retries the share fetch exactly once before giving up.
- Each request log includes `cache_hit` and `ttwid_refreshed` fields. A wave of `ttwid_refreshed=true` followed by `code:2002` is the signal that ttwid alone is no longer sufficient.
```

- [ ] **Step 2: Final acceptance commands**

Run, in order:

```bash
go test ./... -race -count=1
go vet ./...
go build -o server ./cmd/server
```

All three succeed silently.

- [ ] **Step 3: Real-link smoke test — confirm cache hit on second request**

```bash
PORT=18080 ./server > /tmp/sharelink.log 2>&1 &
sleep 2
echo "--- request 1 (cold) ---"
curl -s -X POST http://localhost:18080/api/parse \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://v.douyin.com/R2mGwrMjlLw/"}' | python3 -m json.tool
echo "--- request 2 (warm, should hit cache) ---"
curl -s -X POST http://localhost:18080/api/parse \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://v.douyin.com/R2mGwrMjlLw/"}' | python3 -m json.tool
echo "--- server log (durations + cache_hit) ---"
grep '"msg":"parse"' /tmp/sharelink.log | python3 -c '
import sys, json
for line in sys.stdin:
    d = json.loads(line)
    print("  duration={:>5}ms  cache_hit={}  ttwid_refreshed={}  code={}".format(
        d["duration_ms"], d["cache_hit"], d["ttwid_refreshed"], d["code"]))
'
pkill -f '^./server$'
wait 2>/dev/null
```

Expected:
- Both requests return `code: 0` with the same `play_url`.
- Request 1: `cache_hit=false`, `duration_ms` around 500–1500.
- Request 2: `cache_hit=true`, `duration_ms` significantly lower (<100ms typical) — only the resolve step ran, the share request was skipped.
- Server log shows one `"ttwid acquired"` line at startup.

If request 2 does NOT show `cache_hit=true`, STOP — the cache wiring is wrong.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document ttwid + cache behavior"
```

---

## Out of Scope (Reminder)

Per spec §1, these are intentionally NOT in this plan:
- Persistent cache across restarts
- Cache warming or active invalidation
- Other cookies (`msToken`, `s_v_web_id`, etc.)
- Proxy pool / IP rotation
- Configurable TTL/capacity (hardcoded constants are intentional)
- New environment variables
