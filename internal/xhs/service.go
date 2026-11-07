// Package xhs extracts watermark-free media from Xiaohongshu (小红书 / RED)
// share links.
//
// Flow:
//
//  1. Find a Xiaohongshu link in the share text: a xhslink.com short link or a
//     canonical xiaohongshu.com note URL.
//  2. GET it with a redirect-following client. A xhslink.com short link
//     redirects to the canonical note URL carrying the xsec_token Xiaohongshu
//     now requires; the redirect target's body is the note page itself, so one
//     request yields both. A cookie jar carries the anti-bot cookies set during
//     the redirect through to the final request.
//  3. The note page SSRs window.__INITIAL_STATE__, a JSON blob holding the
//     note's media. Video notes expose a directly playable, watermark-free
//     stream masterUrl; image notes expose the clean full-resolution images.
//
// Watermarks on Xiaohongshu are applied by the app at display time; the source
// URLs in __INITIAL_STATE__ are already watermark-free, so no URL rewriting is
// needed (unlike Douyin's playwm→play swap).
//
// Note URLs require a valid xsec_token. Share (xhslink.com) links carry one
// through the redirect; a bare xiaohongshu.com link pasted without a token will
// return the SPA shell with no note data, surfacing as ErrNoStream.
package xhs

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

const (
	referer = "https://www.xiaohongshu.com/"

	// desktopUA is a desktop Chrome UA. Xiaohongshu SSRs __INITIAL_STATE__ most
	// reliably for desktop web clients.
	desktopUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

	timeout = 10 * time.Second
)

// Result is the Xiaohongshu-specific extraction result. The router maps it into
// the platform-neutral extract.Result. Video notes set PlayURL; image notes set
// Images.
type Result struct {
	NoteID  string
	Title   string
	PlayURL string
	Images  []string
}

// Service resolves Xiaohongshu share links to watermark-free media.
type Service struct {
	client *http.Client
}

// NewService returns a Service backed by a redirect-following client with a
// cookie jar, so anti-bot cookies set while resolving a xhslink.com short link
// persist through to the note-page request.
func NewService() *Service {
	jar, _ := cookiejar.New(nil)
	return &Service{client: &http.Client{Timeout: timeout, Jar: jar}}
}

// Extract finds the first Xiaohongshu link in input and returns its
// watermark-free media (a video PlayURL or an image list).
func (s *Service) Extract(input string) (Result, error) {
	link := linkRe.FindString(input)
	if link == "" {
		return Result{}, ErrNoVideoURL
	}

	body, err := s.fetchNotePage(link)
	if err != nil {
		return Result{}, err
	}

	media, err := parseInitialState(body)
	if err != nil {
		return Result{}, err
	}
	return Result{
		NoteID:  media.NoteID,
		Title:   media.Title,
		PlayURL: media.PlayURL,
		Images:  media.Images,
	}, nil
}

// fetchNotePage GETs the link (following the xhslink.com redirect if needed)
// and returns the note-page HTML body.
func (s *Service) fetchNotePage(link string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, link, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("User-Agent", desktopUA)
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: do request: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: unexpected status %d", ErrUpstream, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrUpstream, err)
	}
	return body, nil
}
