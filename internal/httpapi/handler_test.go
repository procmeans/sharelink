package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/procmeans/sharelink/internal/douyin"
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

func stubExtract(url string, err error) ExtractFunc {
	return func(input string) (string, error) {
		if err != nil {
			return "", err
		}
		return url, nil
	}
}

func mustDecode(t *testing.T, r io.Reader, v any) {
	t.Helper()
	if err := json.NewDecoder(r).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
