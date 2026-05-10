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
