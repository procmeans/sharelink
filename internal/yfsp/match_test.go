package yfsp

import "testing"

func TestMatches(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://www.yfsp.tv/play/pWhVSfzEJbT?id=d87w3SbmpT9", true},
		{"watch https://www.yfsp.tv/play/pWhVSfzEJbT?id=d87w3SbmpT9 now", true},
		{"https://m.yfsp.tv/play/pWhVSfzEJbT?id=d87w3SbmpT9", true},
		{"https://yfsp.tv/play/pWhVSfzEJbT?ref=x&id=d87w3SbmpT9", true},
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

func TestExtract_BuildsMasterURL(t *testing.T) {
	res, err := NewService().Extract("https://www.yfsp.tv/play/pWhVSfzEJbT?id=d87w3SbmpT9")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.ID != "d87w3SbmpT9" {
		t.Errorf("ID = %q, want d87w3SbmpT9", res.ID)
	}
	want := "https://upload.yfsp.tv/api/video/MasterPlayList?id=d87w3SbmpT9"
	if res.PlayURL != want {
		t.Errorf("PlayURL = %q, want %q", res.PlayURL, want)
	}
}
