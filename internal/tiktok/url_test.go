package tiktok

import (
	"errors"
	"testing"
)

func TestExtractURL(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{
			name:  "full video url",
			input: "https://www.tiktok.com/@ricoanimations0/video/7560340173955878155",
			want:  "https://www.tiktok.com/@ricoanimations0/video/7560340173955878155",
		},
		{
			name:  "full video url with query and surrounding text",
			input: "look https://www.tiktok.com/@a.b_c/video/123?is_from_webapp=1&sender_device=pc lol",
			want:  "https://www.tiktok.com/@a.b_c/video/123",
		},
		{
			name:  "mobile host",
			input: "https://m.tiktok.com/@user/video/42",
			want:  "https://m.tiktok.com/@user/video/42",
		},
		{
			name:  "vm short link",
			input: "https://vm.tiktok.com/ZMabc123/",
			want:  "https://vm.tiktok.com/ZMabc123/",
		},
		{
			name:  "vt short link",
			input: "Check this out https://vt.tiktok.com/ZSxyz789/ !",
			want:  "https://vt.tiktok.com/ZSxyz789/",
		},
		{
			name:  "www /t/ short link",
			input: "https://www.tiktok.com/t/ZTabc/",
			want:  "https://www.tiktok.com/t/ZTabc/",
		},
		{
			name:    "douyin link",
			input:   "https://v.douyin.com/iABCxyz/",
			wantErr: ErrNoURL,
		},
		{
			name:    "tiktok profile, not a video",
			input:   "https://www.tiktok.com/@ricoanimations0",
			wantErr: ErrNoURL,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: ErrNoURL,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractURL(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want err %v, got %v (url %q)", tc.wantErr, err, got)
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
