package extract

import "testing"

func TestRouter_UnsupportedURL(t *testing.T) {
	r := NewRouter(nil, nil, nil, nil)
	_, err := r.Extract("just some text with no links")
	if err != ErrUnsupportedURL {
		t.Fatalf("want ErrUnsupportedURL, got %v", err)
	}
}

func TestRouter_NilProvidersSkipped(t *testing.T) {
	// Even a recognizable YouTube link routes to ErrUnsupportedURL when no
	// provider is wired, rather than panicking on a nil service.
	r := NewRouter(nil, nil, nil, nil)
	if _, err := r.Extract("https://youtu.be/ElLTyRLLRpg"); err != ErrUnsupportedURL {
		t.Fatalf("want ErrUnsupportedURL for nil youtube provider, got %v", err)
	}
}
