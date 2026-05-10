package douyin

import "testing"

func TestStripWatermark(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "playwm in path",
			in:   "https://aweme.snssdk.com/aweme/v1/playwm/?video_id=v01",
			want: "https://aweme.snssdk.com/aweme/v1/play/?video_id=v01",
		},
		{
			name: "already no watermark",
			in:   "https://aweme.snssdk.com/aweme/v1/play/?video_id=v01",
			want: "https://aweme.snssdk.com/aweme/v1/play/?video_id=v01",
		},
		{
			name: "only first occurrence is replaced",
			in:   "https://example.com/playwm/?ref=playwm",
			want: "https://example.com/play/?ref=playwm",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StripWatermark(tc.in)
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}
