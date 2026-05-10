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
