package yfsp

import (
	"encoding/json"
	"testing"
)

func TestMatches(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://www.yfsp.tv/play/pWhVSfzEJbT?id=d87w3SbmpT9", true},
		{"watch https://www.yfsp.tv/play/pWhVSfzEJbT?id=d87w3SbmpT9 now", true},
		{"https://m.yfsp.tv/play/pWhVSfzEJbT?id=d87w3SbmpT9", true},
		{"https://yfsp.tv/play/pWhVSfzEJbT?ref=x&id=d87w3SbmpT9", true},
		{"https://www.ayf.tv/play/pWhVSfzEJbT?id=d87w3SbmpT9", true},
		{"https://m.ayf.tv/play/pWhVSfzEJbT?id=d87w3SbmpT9", true},
		{"https://ayf.tv/play/pWhVSfzEJbT?ref=x&id=d87w3SbmpT9", true},
		{"https://www.ayf.tv/play/pWhVSfzEJbT", false}, // no id
		{"https://www.yfsp.tv/play/pWhVSfzEJbT", false}, // no id
		{"https://www.yfsp.tv/play/pWhVSfzEJbT?p=1", false},
		{"https://www.bilibili.com/video/BV1esV8zRETW/", false},
		{"https://www.youtube.com/watch?v=ElLTyRLLRpg", false},
		{"just some text", false},
	}
	for _, c := range cases {
		if got := Matches(c.in); got != c.want {
			t.Errorf("Matches(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIDFromURL(t *testing.T) {
	cases := []struct {
		in     string
		wantID string
		wantOK bool
	}{
		{"https://www.yfsp.tv/play/pWhVSfzEJbT?id=d87w3SbmpT9", "d87w3SbmpT9", true},
		{"https://www.yfsp.tv/play/pWhVSfzEJbT?ref=x&id=d87w3SbmpT9", "d87w3SbmpT9", true},
		{"https://www.yfsp.tv/play/pWhVSfzEJbT?p=1", "", false},
	}
	for _, c := range cases {
		id, ok := idFromURL(c.in)
		if id != c.wantID || ok != c.wantOK {
			t.Errorf("idFromURL(%q) = (%q, %v), want (%q, %v)", c.in, id, ok, c.wantID, c.wantOK)
		}
	}
}

func TestExtract_NoLink(t *testing.T) {
	if _, err := NewService().Extract("no link here"); err != ErrNoVideoURL {
		t.Fatalf("want ErrNoVideoURL, got %v", err)
	}
}

// TestBestStream models the real video/play clarity shape: HD entries are
// VIP-locked (disabled, no URL); the standard-definition entry is playable.
func TestBestStream(t *testing.T) {
	const body = `[
		{"title":"1080","bitrate":1080000,"isVIP":true,"isEnabled":false,"path":null},
		{"title":"720","bitrate":720000,"isVIP":true,"isEnabled":false,"path":null},
		{"title":"576","bitrate":576000,"isVIP":false,"isEnabled":true,"path":{"isHls":true,"result":"https://cdn.example/chunklist.m3u8?t=1"}}
	]`
	var clarities []clarityEntry
	if err := json.Unmarshal([]byte(body), &clarities); err != nil {
		t.Fatal(err)
	}
	url, quality := bestStream(clarities)
	if url != "https://cdn.example/chunklist.m3u8?t=1" {
		t.Errorf("url = %q", url)
	}
	if quality != "576" {
		t.Errorf("quality = %q, want 576", quality)
	}
}

func TestBestStream_PrefersHighestEnabledBitrate(t *testing.T) {
	clarities := []clarityEntry{
		{Title: "576", Bitrate: 576000, IsEnabled: true, Path: &struct {
			IsHls  bool   `json:"isHls"`
			Result string `json:"result"`
		}{true, "sd.m3u8"}},
		{Title: "1080", Bitrate: 1080000, IsEnabled: true, Path: &struct {
			IsHls  bool   `json:"isHls"`
			Result string `json:"result"`
		}{true, "hd.m3u8"}},
	}
	url, quality := bestStream(clarities)
	if url != "hd.m3u8" || quality != "1080" {
		t.Errorf("got (%q,%q), want (hd.m3u8,1080)", url, quality)
	}
}

func TestBestStream_NoneEnabled(t *testing.T) {
	clarities := []clarityEntry{
		{Title: "1080", Bitrate: 1080000, IsEnabled: false},
	}
	if url, _ := bestStream(clarities); url != "" {
		t.Errorf("want empty url when nothing enabled, got %q", url)
	}
}
