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

func TestTTWid_Refresh302WithCookie(t *testing.T) {
	// Reproduces the douyin.com behavior: cookie set on a 302 response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "ttwid", Value: "from302"})
		w.Header().Set("Location", "/home")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	m := newTTWidManagerWithURL(httpclient.NewNoRedirect(), srv.URL)
	if err := m.Refresh(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if m.Cookie() != "from302" {
		t.Fatalf("want from302, got %q", m.Cookie())
	}
}
