package xhs

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtract_NoLink(t *testing.T) {
	if _, err := NewService().Extract("no xiaohongshu link here"); err != ErrNoVideoURL {
		t.Fatalf("want ErrNoVideoURL, got %v", err)
	}
}

func TestFetchNotePage_Headers(t *testing.T) {
	var gotUA, gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotReferer = r.Header.Get("Referer")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, videoPage)
	}))
	defer srv.Close()

	body, err := NewService().fetchNotePage(srv.URL)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotUA != desktopUA {
		t.Errorf("User-Agent = %q, want desktop UA", gotUA)
	}
	if gotReferer != referer {
		t.Errorf("Referer = %q, want %q", gotReferer, referer)
	}
	if media, err := parseInitialState(body); err != nil || media.PlayURL == "" {
		t.Errorf("parse of fetched body failed: %v (%+v)", err, media)
	}
}

func TestFetchNotePage_Upstream5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := NewService().fetchNotePage(srv.URL)
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("want ErrUpstream, got %v", err)
	}
}
