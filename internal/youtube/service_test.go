package youtube

import (
	"testing"

	yt "github.com/kkdai/youtube/v2"
)

func TestMatches(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://www.youtube.com/watch?v=ElLTyRLLRpg", true},
		{"watch here: https://youtube.com/watch?v=ElLTyRLLRpg !!", true},
		{"https://youtu.be/ElLTyRLLRpg?t=10", true},
		{"https://www.youtube.com/shorts/ElLTyRLLRpg", true},
		{"https://m.youtube.com/watch?feature=share&v=ElLTyRLLRpg", true},
		{"https://www.youtube.com/embed/ElLTyRLLRpg", true},
		{"https://v.douyin.com/iABCxxx/", false},
		{"just some text", false},
		{"https://www.youtube.com/", false}, // no video id
	}
	for _, c := range cases {
		if got := Matches(c.in); got != c.want {
			t.Errorf("Matches(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestBestVideo_PrefersHighestResVideoOnly(t *testing.T) {
	fl := yt.FormatList{
		{ItagNo: 18, MimeType: "video/mp4", Width: 640, Height: 360, Bitrate: 500_000, AudioChannels: 2}, // muxed — skip
		{ItagNo: 137, MimeType: "video/mp4", Width: 1920, Height: 1080, Bitrate: 4_000_000},              // best
		{ItagNo: 136, MimeType: "video/mp4", Width: 1280, Height: 720, Bitrate: 2_000_000},
		{ItagNo: 140, MimeType: "audio/mp4", Bitrate: 128_000}, // audio — skip
	}
	best := bestVideo(fl, false)
	if best == nil || best.ItagNo != 137 {
		t.Fatalf("want itag 137 (1080p video-only), got %+v", best)
	}
}

func TestBestVideo_H264OnlyPrefersAvc1(t *testing.T) {
	fl := yt.FormatList{
		{ItagNo: 313, MimeType: `video/webm; codecs="vp9"`, Height: 2160, Bitrate: 20_000_000},       // 4K VP9 — skip in h264 mode
		{ItagNo: 137, MimeType: `video/mp4; codecs="avc1.640028"`, Height: 1080, Bitrate: 4_000_000}, // best avc1
		{ItagNo: 136, MimeType: `video/mp4; codecs="avc1.4d401f"`, Height: 720, Bitrate: 2_000_000},
	}
	if best := bestVideo(fl, true); best == nil || best.ItagNo != 137 {
		t.Fatalf("h264Only: want itag 137 (1080p avc1), got %+v", best)
	}
	if best := bestVideo(fl, false); best == nil || best.ItagNo != 313 {
		t.Fatalf("default: want itag 313 (4K vp9), got %+v", best)
	}
}

func TestBestAudio_PrefersHighestBitrateAudioOnly(t *testing.T) {
	fl := yt.FormatList{
		{ItagNo: 18, MimeType: "video/mp4", Height: 360, Bitrate: 500_000, AudioChannels: 2},
		{ItagNo: 139, MimeType: "audio/mp4", Bitrate: 48_000},
		{ItagNo: 140, MimeType: "audio/mp4", Bitrate: 128_000}, // best
		{ItagNo: 137, MimeType: "video/mp4", Height: 1080, Bitrate: 4_000_000},
	}
	best := bestAudio(fl, false)
	if best == nil || best.ItagNo != 140 {
		t.Fatalf("want itag 140 (128k audio-only), got %+v", best)
	}
}

func TestBestAudio_AACOnlyPrefersMp4a(t *testing.T) {
	fl := yt.FormatList{
		{ItagNo: 251, MimeType: `audio/webm; codecs="opus"`, Bitrate: 160_000}, // opus, higher bitrate — skip in aac mode
		{ItagNo: 140, MimeType: `audio/mp4; codecs="mp4a.40.2"`, Bitrate: 128_000},
	}
	if best := bestAudio(fl, true); best == nil || best.ItagNo != 140 {
		t.Fatalf("aacOnly: want itag 140 (mp4a), got %+v", best)
	}
	if best := bestAudio(fl, false); best == nil || best.ItagNo != 251 {
		t.Fatalf("default: want itag 251 (opus, highest bitrate), got %+v", best)
	}
}

func TestBest_NoneWhenMissing(t *testing.T) {
	onlyMuxed := yt.FormatList{{ItagNo: 18, MimeType: "video/mp4", Height: 360, AudioChannels: 2}}
	if bestAudio(onlyMuxed, false) != nil {
		t.Error("bestAudio should be nil when no audio-only stream exists")
	}
	if bestVideo(yt.FormatList{}, false) != nil {
		t.Error("bestVideo should be nil for empty list")
	}
	// h264 mode with only VP9 video available → no match.
	onlyVP9 := yt.FormatList{{ItagNo: 313, MimeType: `video/webm; codecs="vp9"`, Height: 2160}}
	if bestVideo(onlyVP9, true) != nil {
		t.Error("bestVideo(h264Only) should be nil when only VP9 exists")
	}
}

func TestExtract_NoLink(t *testing.T) {
	_, err := NewService(Muxed).Extract("no link here")
	if err != ErrNoVideoURL {
		t.Fatalf("want ErrNoVideoURL, got %v", err)
	}
}

func TestBestMuxed_PrefersHighestResProgressive(t *testing.T) {
	fl := yt.FormatList{
		{ItagNo: 18, MimeType: "video/mp4", Height: 360, Bitrate: 500_000, AudioChannels: 2},
		{ItagNo: 22, MimeType: "video/mp4", Height: 720, Bitrate: 1_500_000, AudioChannels: 2}, // best muxed
		{ItagNo: 137, MimeType: "video/mp4", Height: 1080, Bitrate: 4_000_000},                 // video-only — skip
		{ItagNo: 140, MimeType: "audio/mp4", Bitrate: 128_000},                                 // audio-only — skip
	}
	if best := bestMuxed(fl); best == nil || best.ItagNo != 22 {
		t.Fatalf("want itag 22 (720p muxed), got %+v", best)
	}
	// No progressive stream available → nil.
	if bestMuxed(yt.FormatList{{ItagNo: 137, MimeType: "video/mp4", Height: 1080}}) != nil {
		t.Error("bestMuxed should be nil when no progressive stream exists")
	}
}
