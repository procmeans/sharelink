package douyin

import (
	"errors"

	"github.com/procmeans/sharelink/internal/cache"
	"github.com/procmeans/sharelink/internal/httpclient"
)

// Result is what Extract returns: the watermark-free play URL plus the
// numeric video id (useful for logging/correlation).
type Result struct {
	PlayURL        string
	VideoID        string
	CacheHit       bool
	TTWidRefreshed bool
}

// Config lets tests inject a fake short URL and share base.
type Config struct {
	ShortURL     string
	ShareBaseURL string
}

// Service orchestrates the extraction pipeline with ttwid cookies and a
// video_id-keyed cache.
type Service struct {
	ttwid *TTWidManager
	cache *cache.Cache
}

func NewService(ttwid *TTWidManager, c *cache.Cache) *Service {
	return &Service{ttwid: ttwid, cache: c}
}

func (s *Service) cookie() string {
	if s.ttwid == nil {
		return ""
	}
	return s.ttwid.Cookie()
}

func (s *Service) refresh() error {
	if s.ttwid == nil {
		return errors.New("ttwid manager not configured")
	}
	return s.ttwid.Refresh()
}

// Extract is the high-level entry point: take user-pasted share text or a
// short URL and return a watermark-free mp4 URL plus the video id.
func (s *Service) Extract(input string) (Result, error) {
	short, err := ExtractShortURL(input)
	if err != nil {
		return Result{}, err
	}
	return s.ExtractWithConfig(Config{ShortURL: short})
}

// ExtractWithConfig runs resolve → cache → fetch → optional refresh+retry → strip.
func (s *Service) ExtractWithConfig(cfg Config) (Result, error) {
	if cfg.ShareBaseURL == "" {
		cfg.ShareBaseURL = shareBaseURL
	}

	videoID, err := ResolveVideoID(httpclient.NewNoRedirect(), s.cookie(), cfg.ShortURL)
	if err != nil {
		return Result{}, err
	}

	if cached, ok := s.cache.Get(videoID); ok {
		return Result{PlayURL: cached, VideoID: videoID, CacheHit: true}, nil
	}

	shareClient := httpclient.NewFollowing()
	playWM, err := fetchPlayURLFromBase(shareClient, s.cookie(), cfg.ShareBaseURL, videoID)
	refreshed := false
	if errors.Is(err, ErrNoPlayURL) {
		refreshed = true
		if rerr := s.refresh(); rerr != nil {
			return Result{VideoID: videoID, TTWidRefreshed: refreshed}, err
		}
		playWM, err = fetchPlayURLFromBase(shareClient, s.cookie(), cfg.ShareBaseURL, videoID)
	}
	if err != nil {
		return Result{VideoID: videoID, TTWidRefreshed: refreshed}, err
	}

	playURL := StripWatermark(playWM)
	s.cache.Set(videoID, playURL)
	return Result{PlayURL: playURL, VideoID: videoID, TTWidRefreshed: refreshed}, nil
}
