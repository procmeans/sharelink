package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/procmeans/sharelink/internal/douyin"
	"github.com/procmeans/sharelink/internal/extract"
)

func TestParseHandler_BadJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")

	NewHandler(stubExtract("", nil)).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	var got map[string]any
	mustDecode(t, rec.Body, &got)
	if int(got["code"].(float64)) != 1001 {
		t.Fatalf("want code 1001, got %v", got["code"])
	}
}

func TestParseHandler_MissingURL(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	NewHandler(stubExtract("", nil)).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestParseHandler_NoShortURL(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{"url":"hi"}`))
	req.Header.Set("Content-Type", "application/json")

	NewHandler(stubExtract("", douyin.ErrNoShortURL)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", rec.Code)
	}
	var got map[string]any
	mustDecode(t, rec.Body, &got)
	if int(got["code"].(float64)) != 1002 {
		t.Fatalf("want code 1002, got %v", got["code"])
	}
}

func TestParseHandler_InvalidRedirect(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/parse",
		strings.NewReader(`{"url":"https://v.douyin.com/x/"}`))
	req.Header.Set("Content-Type", "application/json")

	NewHandler(stubExtract("", douyin.ErrInvalidRedirect)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", rec.Code)
	}
	var got map[string]any
	mustDecode(t, rec.Body, &got)
	if int(got["code"].(float64)) != 1003 {
		t.Fatalf("want code 1003, got %v", got["code"])
	}
}

func TestParseHandler_Upstream(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/parse",
		strings.NewReader(`{"url":"https://v.douyin.com/x/"}`))
	req.Header.Set("Content-Type", "application/json")

	NewHandler(stubExtract("", douyin.ErrUpstream)).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", rec.Code)
	}
	var got map[string]any
	mustDecode(t, rec.Body, &got)
	if int(got["code"].(float64)) != 2001 {
		t.Fatalf("want code 2001, got %v", got["code"])
	}
}

func TestParseHandler_NoPlayURL(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/parse",
		strings.NewReader(`{"url":"https://v.douyin.com/x/"}`))
	req.Header.Set("Content-Type", "application/json")

	NewHandler(stubExtract("", douyin.ErrNoPlayURL)).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", rec.Code)
	}
	var got map[string]any
	mustDecode(t, rec.Body, &got)
	if int(got["code"].(float64)) != 2002 {
		t.Fatalf("want code 2002, got %v", got["code"])
	}
}

func TestParseHandler_Success(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/parse",
		strings.NewReader(`{"url":"https://v.douyin.com/x/"}`))
	req.Header.Set("Content-Type", "application/json")

	NewHandler(stubExtract("https://cdn.example.com/play/v.mp4", nil)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var got map[string]any
	mustDecode(t, rec.Body, &got)
	if int(got["code"].(float64)) != 0 {
		t.Fatalf("want code 0, got %v", got["code"])
	}
	if got["play_url"] != "https://cdn.example.com/play/v.mp4" {
		t.Fatalf("unexpected play_url: %v", got["play_url"])
	}
}

func TestParseHandler_MethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/parse", nil)

	NewHandler(stubExtract("", nil)).ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
	var got map[string]any
	mustDecode(t, rec.Body, &got)
	if int(got["code"].(float64)) != 1004 {
		t.Fatalf("want code 1004, got %v", got["code"])
	}
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	NewHandler(stubExtract("", nil)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ok") {
		t.Fatalf("want body to contain ok, got %q", rec.Body.String())
	}
}

// helpers

func stubExtract(playURL string, err error) ExtractFunc {
	return func(input string) (extract.Result, error) {
		if err != nil {
			return extract.Result{}, err
		}
		return extract.Result{Source: "douyin", PlayURL: playURL}, nil
	}
}

func mustDecode(t *testing.T, r io.Reader, v any) {
	t.Helper()
	if err := json.NewDecoder(r).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
