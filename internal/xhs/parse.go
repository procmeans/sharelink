package xhs

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// initialStateMarker precedes the JSON object Xiaohongshu inlines into the note
// page for SSR'd state. The assignment has no spaces:
// `window.__INITIAL_STATE__={...}`.
var initialStateMarker = []byte("__INITIAL_STATE__=")

// scriptClose bounds the inlined JSON; we slice up to it before sanitizing.
var scriptClose = []byte("</script>")

// originVideoBase builds a watermark-free video URL from a video's
// originVideoKey when the stream block is absent.
const originVideoBase = "https://sns-video-bd.xhscdn.com/"

// noteMedia is the platform-neutral result of parsing a note page: either a
// single video PlayURL (video notes) or a list of image URLs (image notes).
type noteMedia struct {
	NoteID  string
	Title   string
	Type    string // "video" or "normal"
	PlayURL string
	Images  []string
}

// initialState models the subset of window.__INITIAL_STATE__ we consume. Field
// names follow Xiaohongshu's web payload.
type initialState struct {
	Note struct {
		FirstNoteID   string              `json:"firstNoteId"`
		NoteDetailMap map[string]noteWrap `json:"noteDetailMap"`
	} `json:"note"`
}

type noteWrap struct {
	Note noteDetail `json:"note"`
}

type noteDetail struct {
	NoteID string `json:"noteId"`
	Title  string `json:"title"`
	Type   string `json:"type"`
	Video  struct {
		Media struct {
			Stream streamSet `json:"stream"`
		} `json:"media"`
		Consumer struct {
			OriginVideoKey string `json:"originVideoKey"`
		} `json:"consumer"`
	} `json:"video"`
	ImageList []imageItem `json:"imageList"`
}

// streamSet lists the codec-specific stream variants. Each carries a directly
// playable, watermark-free masterUrl.
type streamSet struct {
	H264 []streamVariant `json:"h264"`
	H265 []streamVariant `json:"h265"`
	H266 []streamVariant `json:"h266"`
	AV1  []streamVariant `json:"av1"`
}

type streamVariant struct {
	MasterURL string `json:"masterUrl"`
}

type imageItem struct {
	URLDefault string `json:"urlDefault"`
	InfoList   []struct {
		Scene string `json:"imageScene"`
		URL   string `json:"url"`
	} `json:"infoList"`
}

// parseInitialState extracts watermark-free media from a Xiaohongshu note page.
// It locates the inlined window.__INITIAL_STATE__ JSON, sanitizes the bare
// `undefined` values Xiaohongshu emits (invalid JSON) into null, decodes it,
// and selects the note's video stream or image list.
func parseInitialState(body []byte) (noteMedia, error) {
	idx := bytes.Index(body, initialStateMarker)
	if idx < 0 {
		return noteMedia{}, fmt.Errorf("%w: __INITIAL_STATE__ not found", ErrNoStream)
	}
	rest := body[idx+len(initialStateMarker):]

	// Bound the JSON at the closing </script> so the undefined→null
	// sanitization (and decode) operate on a well-defined span.
	if end := bytes.Index(rest, scriptClose); end >= 0 {
		rest = rest[:end]
	}

	// Xiaohongshu inlines raw JS `undefined` as a value, which is not valid
	// JSON. Replace it with null. The media fields we read (URLs, type) never
	// legitimately contain this token, so the blunt replace is safe here.
	rest = bytes.ReplaceAll(rest, []byte("undefined"), []byte("null"))

	start := bytes.IndexByte(rest, '{')
	if start < 0 {
		return noteMedia{}, fmt.Errorf("%w: __INITIAL_STATE__ object not found", ErrNoStream)
	}

	var st initialState
	dec := json.NewDecoder(bytes.NewReader(rest[start:]))
	if err := dec.Decode(&st); err != nil {
		return noteMedia{}, fmt.Errorf("%w: decode __INITIAL_STATE__: %v", ErrNoStream, err)
	}

	detail, ok := pickNote(st)
	if !ok {
		return noteMedia{}, fmt.Errorf("%w: empty noteDetailMap", ErrNoStream)
	}

	media := noteMedia{
		NoteID: detail.NoteID,
		Title:  detail.Title,
		Type:   detail.Type,
	}

	if url := videoURL(detail); url != "" {
		media.PlayURL = url
		return media, nil
	}
	if imgs := imageURLs(detail); len(imgs) > 0 {
		media.Images = imgs
		return media, nil
	}
	return noteMedia{}, fmt.Errorf("%w: note has neither video stream nor images", ErrNoStream)
}

// pickNote returns the note detail keyed by firstNoteId, falling back to any
// entry in the map.
func pickNote(st initialState) (noteDetail, bool) {
	m := st.Note.NoteDetailMap
	if len(m) == 0 {
		return noteDetail{}, false
	}
	if w, ok := m[st.Note.FirstNoteID]; ok {
		return w.Note, true
	}
	for _, w := range m {
		return w.Note, true
	}
	return noteDetail{}, false
}

// videoURL returns the watermark-free video URL for a note, preferring the
// H.264 stream (most broadly playable) and falling back through the other
// codecs, then to a URL built from originVideoKey.
func videoURL(d noteDetail) string {
	for _, variants := range [][]streamVariant{
		d.Video.Media.Stream.H264,
		d.Video.Media.Stream.H265,
		d.Video.Media.Stream.AV1,
		d.Video.Media.Stream.H266,
	} {
		for _, v := range variants {
			if v.MasterURL != "" {
				return v.MasterURL
			}
		}
	}
	if key := d.Video.Consumer.OriginVideoKey; key != "" {
		return originVideoBase + key
	}
	return ""
}

// imageURLs returns the watermark-free image URLs for an image note. urlDefault
// is the clean, full-resolution image; if absent we fall back to the first
// infoList entry.
func imageURLs(d noteDetail) []string {
	var out []string
	for _, img := range d.ImageList {
		if img.URLDefault != "" {
			out = append(out, img.URLDefault)
			continue
		}
		if len(img.InfoList) > 0 && img.InfoList[0].URL != "" {
			out = append(out, img.InfoList[0].URL)
		}
	}
	return out
}
