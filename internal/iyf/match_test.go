package iyf

import (
	"encoding/json"
	"testing"
)

func TestMatches(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://mview.iyf.tv/play/qCcsYzzKZ76", true},
		{"watch https://mview.iyf.tv/play/qCcsYzzKZ76 now", true},
		{"https://www.iyf.tv/play/qCcsYzzKZ76", true},
		{"https://m.iyf.tv/play/qCcsYzzKZ76", true},
		{"https://iyf.tv/play/qCcsYzzKZ76", true},
		{"https://mview.iyf.tv/play/qCcsYzzKZ76?from=share", true},
		{"https://www.yfsp.tv/play/pWhVSfzEJbT?id=d87w3SbmpT9", false},
		{"https://www.iyf.tv/home", false},
		{"https://www.bilibili.com/video/BV1esV8zRETW/", false},
		{"just some text", false},
	}
	for _, c := range cases {
		if got := Matches(c.in); got != c.want {
			t.Errorf("Matches(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestKeyFromURL(t *testing.T) {
	cases := []struct {
		in      string
		wantKey string
		wantOK  bool
	}{
		{"https://mview.iyf.tv/play/qCcsYzzKZ76", "qCcsYzzKZ76", true},
		{"see https://www.iyf.tv/play/abc123?x=1 here", "abc123", true},
		{"no link here", "", false},
	}
	for _, c := range cases {
		key, ok := keyFromURL(c.in)
		if key != c.wantKey || ok != c.wantOK {
			t.Errorf("keyFromURL(%q) = (%q, %v), want (%q, %v)", c.in, key, ok, c.wantKey, c.wantOK)
		}
	}
}

func TestExtract_NoLink(t *testing.T) {
	if _, err := NewService().Extract("no link here"); err != ErrNoVideoURL {
		t.Fatalf("want ErrNoVideoURL, got %v", err)
	}
}

// TestBestStream models the real getPlayData clarity shape: HD entries are
// VIP-locked with no mediaUrl; the standard-definition entry is playable.
func TestBestStream(t *testing.T) {
	const body = `[
		{"title":"哪吒之魔童降世","isVip":true,"resolution":"1080","resolutionDes":"1080P","mediaUrl":""},
		{"title":"哪吒之魔童降世","isVip":true,"resolution":"720","resolutionDes":"720P","mediaUrl":""},
		{"title":"哪吒之魔童降世","isVip":false,"resolution":"480","resolutionDes":"480P","mediaUrl":"https://cdn.example/chunklist.m3u8?t=1"}
	]`
	var episodes []episode
	if err := json.Unmarshal([]byte(body), &episodes); err != nil {
		t.Fatal(err)
	}
	ep := bestStream(episodes)
	if ep == nil {
		t.Fatal("bestStream returned nil")
	}
	if ep.MediaURL != "https://cdn.example/chunklist.m3u8?t=1" {
		t.Errorf("mediaUrl = %q", ep.MediaURL)
	}
	if ep.ResolutionDes != "480P" {
		t.Errorf("quality = %q, want 480P", ep.ResolutionDes)
	}
}

func TestBestStream_PrefersHighestPlayableResolution(t *testing.T) {
	episodes := []episode{
		{Resolution: "480", ResolutionDes: "480P", MediaURL: "sd.m3u8"},
		{Resolution: "1080", ResolutionDes: "1080P", MediaURL: "hd.m3u8"},
	}
	ep := bestStream(episodes)
	if ep == nil || ep.MediaURL != "hd.m3u8" || ep.ResolutionDes != "1080P" {
		t.Errorf("got %+v, want hd.m3u8/1080P", ep)
	}
}

func TestBestStream_NonePlayable(t *testing.T) {
	episodes := []episode{
		{Resolution: "1080", ResolutionDes: "1080P", IsVip: true, MediaURL: ""},
	}
	if ep := bestStream(episodes); ep != nil {
		t.Errorf("want nil when nothing playable, got %+v", ep)
	}
}
