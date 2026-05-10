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
