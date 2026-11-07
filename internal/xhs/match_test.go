package xhs

import "testing"

func TestMatches(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"http://xhslink.com/o/4rqfq3LLTNT", true},
		{"https://xhslink.com/a/abc123", true},
		{"看看这个 http://xhslink.com/o/4rqfq3LLTNT 这篇笔记", true},
		{"https://www.xiaohongshu.com/explore/64a1b2c3000000001f00abcd", true},
		{"https://www.xiaohongshu.com/explore/64a1b2c3000000001f00abcd?xsec_token=ABC", true},
		{"https://www.xiaohongshu.com/discovery/item/64a1b2c3000000001f00abcd", true},
		{"https://v.douyin.com/iABCxxx/", false},
		{"https://www.bilibili.com/video/BV1esV8zRETW/", false},
		{"https://www.xiaohongshu.com/", false}, // no note id
		{"just some text", false},
	}
	for _, c := range cases {
		if got := Matches(c.in); got != c.want {
			t.Errorf("Matches(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
