package yfsp

import "testing"

func TestCanonicalQuery(t *testing.T) {
	cases := []struct{ in, want string }{
		{"cinema=1&id=ABC&a=0", "cinema=1&id=ABC&a=0"},
		{"a=Hello+World&b=x%2Fy", "a=Hello World&b=x/y"},
		{"", ""},
	}
	for _, c := range cases {
		if got := canonicalQuery(c.in); got != c.want {
			t.Errorf("canonicalQuery(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Golden values produced by the reference (verified-working) implementation:
// vv = md5(pub + "&" + lower(canonicalQuery(query)) + "&" + priv).
func TestSign(t *testing.T) {
	cases := []struct {
		base, query, pub, priv, wantVV string
	}{
		{"https://m10.yfsp.tv/v3/video/play", "cinema=1&id=ABC&a=0", "PUBKEY", "PRIVKEY", "3523ed4c016b3a82b9ed144c3cbf413a"},
		{"b", "cinema=1&id=Wj6tgcmgDdD&a=0&lang=none&usersign=1&region=GL.&device=1&isMasterSupport=1", "CJSuABC", "SuCZpriv", "e7047e4b7cdb9eb90239f1a31f27f0d4"},
		{"b", "a=Hello+World&b=x%2Fy", "p", "k", "911b5a85d40838b50213b1b3adc9189b"},
	}
	for _, c := range cases {
		got := sign(c.base, c.query, c.pub, c.priv)
		want := c.base + "?" + c.query + "&vv=" + c.wantVV + "&pub=" + c.pub
		if got != want {
			t.Errorf("sign(%q,…) =\n  %q\nwant\n  %q", c.query, got, want)
		}
	}
}
