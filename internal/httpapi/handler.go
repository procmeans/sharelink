package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/procmeans/sharelink/internal/bilibili"
	"github.com/procmeans/sharelink/internal/douyin"
	"github.com/procmeans/sharelink/internal/extract"
	"github.com/procmeans/sharelink/internal/youtube"
)

// ExtractFunc is the dependency injected into the handler so tests can stub
// the extraction logic without touching the real implementation.
type ExtractFunc func(input string) (extract.Result, error)

type parseRequest struct {
	URL string `json:"url"`
}

type successResponse struct {
	Code     int    `json:"code"`
	Source   string `json:"source,omitempty"`
	PlayURL  string `json:"play_url,omitempty"`
	VideoURL string `json:"video_url,omitempty"`
	AudioURL string `json:"audio_url,omitempty"`
	Title    string `json:"title,omitempty"`
}

type errorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewHandler returns the top-level http.Handler for the service.
func NewHandler(extract ExtractFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/api/parse", parseHandler(extract))
	return mux
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

func parseHandler(extract ExtractFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, 1004, "method not allowed")
			return
		}

		var req parseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, 1001, "invalid json body")
			return
		}
		if strings.TrimSpace(req.URL) == "" {
			writeError(w, http.StatusBadRequest, 1001, "url is required")
			return
		}

		res, err := extract(req.URL)
		if err != nil {
			status, code := mapError(err)
			writeError(w, status, code, err.Error())
			logRequest(req.URL, res.VideoID, "", "error", code, res.CacheHit, res.TTWidRefreshed, time.Since(start))
			return
		}

		writeJSON(w, http.StatusOK, successResponse{
			Code:     0,
			Source:   res.Source,
			PlayURL:  res.PlayURL,
			VideoURL: res.VideoURL,
			AudioURL: res.AudioURL,
			Title:    res.Title,
		})
		logRequest(req.URL, res.VideoID, res.PlayURL, "ok", 0, res.CacheHit, res.TTWidRefreshed, time.Since(start))
	}
}

func mapError(err error) (int, int) {
	switch {
	case errors.Is(err, douyin.ErrNoShortURL),
		errors.Is(err, youtube.ErrNoVideoURL),
		errors.Is(err, bilibili.ErrNoVideoURL),
		errors.Is(err, extract.ErrUnsupportedURL):
		return http.StatusUnprocessableEntity, 1002
	case errors.Is(err, douyin.ErrInvalidRedirect):
		return http.StatusUnprocessableEntity, 1003
	case errors.Is(err, douyin.ErrUpstream),
		errors.Is(err, youtube.ErrUpstream),
		errors.Is(err, bilibili.ErrUpstream):
		return http.StatusBadGateway, 2001
	case errors.Is(err, douyin.ErrNoPlayURL),
		errors.Is(err, youtube.ErrNoStream),
		errors.Is(err, bilibili.ErrNoStream):
		return http.StatusBadGateway, 2002
	default:
		return http.StatusInternalServerError, 1000
	}
}

func writeError(w http.ResponseWriter, status, code int, msg string) {
	writeJSON(w, status, errorResponse{Code: code, Message: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func logRequest(input, videoID, playURL, result string, code int, cacheHit, ttwidRefreshed bool, dur time.Duration) {
	in := input
	if len(in) > 100 {
		in = in[:100]
	}
	slog.Info("parse",
		"input", in,
		"video_id", videoID,
		"play_url", playURL,
		"result", result,
		"code", code,
		"cache_hit", cacheHit,
		"ttwid_refreshed", ttwidRefreshed,
		"duration_ms", dur.Milliseconds(),
	)
}
