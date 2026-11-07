package bilibili

import "testing"

func TestMatches(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://www.bilibili.com/video/BV1esV8zRETW/", true},
		{"watch this https://www.bilibili.com/video/BV1esV8zRETW?p=1 cool", true},
		{"https://m.bilibili.com/video/BV1esV8zRETW", true},
		{"https://www.bilibili.com/video/av170001", true},
		{"https://b23.tv/abCD123", true},
		{"https://v.douyin.com/iABCxxx/", false},
		{"https://www.youtube.com/watch?v=ElLTyRLLRpg", false},
		{"https://www.bilibili.com/", false}, // no video id
		{"just some text", false},
	}
	for _, c := range cases {
		if got := Matches(c.in); got != c.want {
			t.Errorf("Matches(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRefFromURL(t *testing.T) {
	cases := []struct {
		in      string
		wantRef string
		wantOK  bool
	}{
		{"https://www.bilibili.com/video/BV1esV8zRETW/", "bvid=BV1esV8zRETW", true},
		{"https://www.bilibili.com/video/av170001?p=2", "aid=170001", true},
		{"https://www.bilibili.com/", "", false},
	}
	for _, c := range cases {
		ref, ok := refFromURL(c.in)
		if ref != c.wantRef || ok != c.wantOK {
			t.Errorf("refFromURL(%q) = (%q, %v), want (%q, %v)", c.in, ref, ok, c.wantRef, c.wantOK)
		}
	}
}

func TestExtract_NoLink(t *testing.T) {
	if _, err := NewService().Extract("no link here"); err != ErrNoVideoURL {
		t.Fatalf("want ErrNoVideoURL, got %v", err)
	}
}
