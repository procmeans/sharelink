package extract

import (
	"errors"
	"testing"

	"github.com/procmeans/sharelink/internal/douyin"
	"github.com/procmeans/sharelink/internal/tiktok"
)

func newRouter(called *string) Router {
	return Router{
		Douyin: func(string) (douyin.Result, error) {
			*called = "douyin"
			return douyin.Result{PlayURL: "dy", VideoID: "1", CacheHit: true, TTWidRefreshed: true}, nil
		},
		TikTok: func(string) (tiktok.Result, error) {
			*called = "tiktok"
			return tiktok.Result{PlayURL: "tt", VideoID: "2", Headers: map[string]string{"Referer": "r"}}, nil
		},
	}
}

func TestRouter_TikTok(t *testing.T) {
	var called string
	res, err := newRouter(&called).Extract("see https://www.tiktok.com/@u/video/2 !")
	if err != nil {
		t.Fatal(err)
	}
	if called != "tiktok" || res.PlayURL != "tt" || res.VideoID != "2" || res.Headers["Referer"] != "r" {
		t.Fatalf("called=%s res=%+v", called, res)
	}
}

func TestRouter_Douyin(t *testing.T) {
	var called string
	res, err := newRouter(&called).Extract("复制打开抖音 https://v.douyin.com/abc/")
	if err != nil {
		t.Fatal(err)
	}
	if called != "douyin" || res.PlayURL != "dy" || !res.CacheHit || !res.TTWidRefreshed || res.Headers != nil {
		t.Fatalf("called=%s res=%+v", called, res)
	}
}

func TestRouter_NoLinkFallsThroughToDouyinError(t *testing.T) {
	r := Router{
		Douyin: func(string) (douyin.Result, error) { return douyin.Result{}, douyin.ErrNoShortURL },
		TikTok: func(string) (tiktok.Result, error) {
			t.Fatal("tiktok should not be called")
			return tiktok.Result{}, nil
		},
	}
	if _, err := r.Extract("just text"); !errors.Is(err, douyin.ErrNoShortURL) {
		t.Fatalf("want ErrNoShortURL, got %v", err)
	}
}
