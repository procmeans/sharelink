package tiktok

import (
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
)

func newJarClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

func samplePage(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/sample_video.html")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFetch_ParsesPlayAddrAndToken(t *testing.T) {
	page := samplePage(t)
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		http.SetCookie(w, &http.Cookie{Name: "tt_chain_token", Value: "tok123", Path: "/"})
		_, _ = w.Write(page)
	}))
	defer srv.Close()

	res, err := fetch(newJarClient(t), srv.URL+"/@u/video/7560340173955878155")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.VideoID != "7560340173955878155" {
		t.Fatalf("video id: got %q", res.VideoID)
	}
	wantPlay := "https://v16-webapp-prime.us.tiktok.com/video/tos/nowm/?a=1988&mime_type=video_mp4&tk=tt_chain_token"
	if res.PlayURL != wantPlay {
		t.Fatalf("play url: got %q", res.PlayURL)
	}
	if res.Headers["Cookie"] != "tt_chain_token=tok123" {
		t.Fatalf("cookie header: got %q", res.Headers["Cookie"])
	}
	if res.Headers["Referer"] != "https://www.tiktok.com/" {
		t.Fatalf("referer header: got %q", res.Headers["Referer"])
	}
	// The parsed JSON key only exists on the desktop page.
	if gotUA != desktopUA {
		t.Fatalf("want desktop UA, got %q", gotUA)
	}
}

func TestFetch_FollowsShortLinkRedirect(t *testing.T) {
	page := samplePage(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/ZSshort/", func(w http.ResponseWriter, r *http.Request) {
		// Token set on the redirect hop must still be picked up.
		http.SetCookie(w, &http.Cookie{Name: "tt_chain_token", Value: "fromredirect", Path: "/"})
		http.Redirect(w, r, "/@u/video/7560340173955878155", http.StatusFound)
	})
	mux.HandleFunc("/@u/video/7560340173955878155", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(page)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := fetch(newJarClient(t), srv.URL+"/ZSshort/")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Headers["Cookie"] != "tt_chain_token=fromredirect" {
		t.Fatalf("cookie header: got %q", res.Headers["Cookie"])
	}
}

func TestFetch_Errors(t *testing.T) {
	page := samplePage(t)
	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
	}{
		{
			name: "non-2xx",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantErr: ErrUpstream,
		},
		{
			name: "marker missing",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.SetCookie(w, &http.Cookie{Name: "tt_chain_token", Value: "t", Path: "/"})
				_, _ = w.Write([]byte("<html>captcha</html>"))
			},
			wantErr: ErrNoPlayURL,
		},
		{
			name: "video unavailable status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.SetCookie(w, &http.Cookie{Name: "tt_chain_token", Value: "t", Path: "/"})
				_, _ = w.Write([]byte(`<script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">` +
					`{"__DEFAULT_SCOPE__":{"webapp.video-detail":{"statusCode":10204,"statusMsg":"item doesn't exist"}}}</script>`))
			},
			wantErr: ErrNoPlayURL,
		},
		{
			name: "no token cookie",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(page)
			},
			wantErr: ErrUpstream,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			_, err := fetch(newJarClient(t), srv.URL+"/@u/video/1")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}
