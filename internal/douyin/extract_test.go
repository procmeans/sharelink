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
//
//	GET /short/* → 302 to /share/video/<id>/
//	GET /share/video/<id>/ → returns fixture HTML
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
		ShortURL:     srv.URL + "/short/abc",
		ShareBaseURL: srv.URL,
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
