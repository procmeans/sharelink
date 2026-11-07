package youtube

import (
	"errors"
	"strings"
	"testing"
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

func TestExtract_NoLink(t *testing.T) {
	_, err := NewService(Muxed).Extract("no link here")
	if err != ErrNoVideoURL {
		t.Fatalf("want ErrNoVideoURL, got %v", err)
	}
}

// stubService returns a Service whose yt-dlp runner records its args and
// returns out/err.
func stubService(q Quality, out string, err error, gotArgs *[]string) *Service {
	return &Service{quality: q, run: func(args ...string) ([]byte, error) {
		*gotArgs = args
		return []byte(out), err
	}}
}

func TestFormatSelector(t *testing.T) {
	cases := map[Quality]string{
		Muxed: "b",
		Best:  "bv+ba",
		H264:  "bv[vcodec^=avc1]+ba[acodec^=mp4a]",
	}
	for q, want := range cases {
		if got := formatSelector(q); got != want {
			t.Errorf("formatSelector(%d) = %q, want %q", q, got, want)
		}
	}
}

func TestExtract_MuxedReadsTopLevelURL(t *testing.T) {
	var args []string
	s := stubService(Muxed, `{"id":"dQw4w9WgXcQ","title":"Rick","url":"https://g/muxed"}`, nil, &args)
	res, err := s.Extract("watch https://youtu.be/dQw4w9WgXcQ?t=3 now")
	if err != nil {
		t.Fatal(err)
	}
	if res.PlayURL != "https://g/muxed" || res.VideoID != "dQw4w9WgXcQ" || res.Title != "Rick" {
		t.Fatalf("unexpected result: %+v", res)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-J") || !strings.Contains(joined, "-f b ") ||
		args[len(args)-1] != "https://youtu.be/dQw4w9WgXcQ" || args[len(args)-2] != "--" {
		t.Fatalf("unexpected yt-dlp args: %q", args)
	}
}

func TestExtract_SeparatedReadsRequestedFormats(t *testing.T) {
	var args []string
	out := `{"id":"x","title":"t","requested_formats":[
		{"url":"https://g/video","vcodec":"avc1.640028","acodec":"none"},
		{"url":"https://g/audio","vcodec":"none","acodec":"mp4a.40.2"}]}`
	res, err := stubService(H264, out, nil, &args).Extract("https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	if err != nil {
		t.Fatal(err)
	}
	if res.VideoURL != "https://g/video" || res.AudioURL != "https://g/audio" || res.PlayURL != "" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestExtract_NoStream(t *testing.T) {
	var args []string
	if _, err := stubService(Muxed, `{"id":"x"}`, nil, &args).Extract("https://youtu.be/dQw4w9WgXcQ"); !errors.Is(err, ErrNoStream) {
		t.Fatalf("muxed: want ErrNoStream, got %v", err)
	}
	onlyVideo := `{"id":"x","requested_formats":[{"url":"https://g/video","vcodec":"vp9","acodec":"none"}]}`
	if _, err := stubService(Best, onlyVideo, nil, &args).Extract("https://youtu.be/dQw4w9WgXcQ"); !errors.Is(err, ErrNoStream) {
		t.Fatalf("separated: want ErrNoStream, got %v", err)
	}
}

func TestExtract_YTDLPFailureIsUpstream(t *testing.T) {
	var args []string
	s := stubService(Muxed, "", errors.New("yt-dlp: exit status 1: ERROR: Sign in to confirm you're not a bot"), &args)
	_, err := s.Extract("https://youtu.be/dQw4w9WgXcQ")
	if !errors.Is(err, ErrUpstream) || !strings.Contains(err.Error(), "not a bot") {
		t.Fatalf("want ErrUpstream with yt-dlp message, got %v", err)
	}
}
