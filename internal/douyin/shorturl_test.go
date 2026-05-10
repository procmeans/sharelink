package douyin

import (
	"errors"
	"testing"
)

func TestExtractShortURL(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{
			name:  "bare short url",
			input: "https://v.douyin.com/iABCxyz/",
			want:  "https://v.douyin.com/iABCxyz/",
		},
		{
			name:  "share text with emoji and trailing slash",
			input: "5.99 fyu:/ 复制打开抖音,看看【某某的作品】https://v.douyin.com/iABCxyz/ 复制此链接",
			want:  "https://v.douyin.com/iABCxyz/",
		},
		{
			name:  "http scheme",
			input: "look at this http://v.douyin.com/abc123 cool",
			want:  "http://v.douyin.com/abc123",
		},
		{
			name:  "no trailing slash",
			input: "see https://v.douyin.com/abc123 here",
			want:  "https://v.douyin.com/abc123",
		},
		{
			name:    "empty",
			input:   "",
			wantErr: ErrNoShortURL,
		},
		{
			name:    "no douyin url",
			input:   "this is just text https://example.com/foo",
			wantErr: ErrNoShortURL,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractShortURL(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want err %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}
