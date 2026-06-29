package xhs

import (
	"errors"
	"testing"
)

const videoPage = `<!doctype html><html><head><script>window.__INITIAL_STATE__={` +
	`"note":{"firstNoteId":"abc123","noteDetailMap":{"abc123":{"note":{` +
	`"noteId":"abc123","type":"video","title":"金泰梨直拍",` +
	`"video":{"media":{"stream":{"h264":[{"masterUrl":"https://sns-video-bd.xhscdn.com/clean.mp4"}]}},` +
	`"consumer":{"originVideoKey":"pre_post/key"}},"imageList":[]}}}}}` +
	`</script></head><body></body></html>`

const imagePage = `<script>window.__INITIAL_STATE__={` +
	`"note":{"firstNoteId":"img1","noteDetailMap":{"img1":{"note":{` +
	`"noteId":"img1","type":"normal","title":"图文笔记",` +
	`"imageList":[{"urlDefault":"https://ci.xhscdn.com/1.jpg"},{"urlDefault":"https://ci.xhscdn.com/2.jpg"}]}}}}}` +
	`</script>`

// originKeyPage has no stream block, only an originVideoKey to fall back to,
// and includes a bare `undefined` value (invalid JSON) that must be sanitized.
const originKeyPage = `<script>window.__INITIAL_STATE__={` +
	`"note":{"firstNoteId":"v2","noteDetailMap":{"v2":{"note":{` +
	`"noteId":"v2","type":"video","title":"x","xsecToken":undefined,` +
	`"video":{"media":{"stream":{"h264":[]}},"consumer":{"originVideoKey":"pre/abc"}},` +
	`"imageList":[]}}}}}</script>`

func TestParseInitialState_Video(t *testing.T) {
	media, err := parseInitialState([]byte(videoPage))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if media.PlayURL != "https://sns-video-bd.xhscdn.com/clean.mp4" {
		t.Errorf("PlayURL = %q", media.PlayURL)
	}
	if len(media.Images) != 0 {
		t.Errorf("Images = %v, want empty for video note", media.Images)
	}
	if media.Title != "金泰梨直拍" || media.NoteID != "abc123" {
		t.Errorf("Title=%q NoteID=%q", media.Title, media.NoteID)
	}
}

func TestParseInitialState_Image(t *testing.T) {
	media, err := parseInitialState([]byte(imagePage))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if media.PlayURL != "" {
		t.Errorf("PlayURL = %q, want empty for image note", media.PlayURL)
	}
	want := []string{"https://ci.xhscdn.com/1.jpg", "https://ci.xhscdn.com/2.jpg"}
	if len(media.Images) != len(want) {
		t.Fatalf("Images = %v, want %v", media.Images, want)
	}
	for i := range want {
		if media.Images[i] != want[i] {
			t.Errorf("Images[%d] = %q, want %q", i, media.Images[i], want[i])
		}
	}
}

func TestParseInitialState_OriginKeyFallbackAndUndefined(t *testing.T) {
	media, err := parseInitialState([]byte(originKeyPage))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if media.PlayURL != originVideoBase+"pre/abc" {
		t.Errorf("PlayURL = %q, want originVideoKey fallback", media.PlayURL)
	}
}

func TestParseInitialState_NoMarker(t *testing.T) {
	_, err := parseInitialState([]byte("<html><body>nothing</body></html>"))
	if !errors.Is(err, ErrNoStream) {
		t.Fatalf("want ErrNoStream, got %v", err)
	}
}

func TestParseInitialState_NoMedia(t *testing.T) {
	const page = `<script>window.__INITIAL_STATE__={"note":{"firstNoteId":"x","noteDetailMap":{"x":{"note":{"noteId":"x","type":"normal","imageList":[]}}}}}</script>`
	_, err := parseInitialState([]byte(page))
	if !errors.Is(err, ErrNoStream) {
		t.Fatalf("want ErrNoStream, got %v", err)
	}
}
