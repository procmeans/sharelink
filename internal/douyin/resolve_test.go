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

		got, err := ResolveVideoID(httpclient.NewNoRedirect(), "", srv.URL)
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

		got, err := ResolveVideoID(httpclient.NewNoRedirect(), "", srv.URL)
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

		_, err := ResolveVideoID(httpclient.NewNoRedirect(), "", srv.URL)
		if !errors.Is(err, ErrInvalidRedirect) {
			t.Fatalf("want ErrInvalidRedirect, got %v", err)
		}
	})

	t.Run("upstream 500", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := ResolveVideoID(httpclient.NewNoRedirect(), "", srv.URL)
		if !errors.Is(err, ErrUpstream) {
			t.Fatalf("want ErrUpstream, got %v", err)
		}
	})
}

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
