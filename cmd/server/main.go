// Package main is the HTTP server entrypoint for the Douyin direct-link service.
package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/procmeans/sharelink/internal/cache"
	"github.com/procmeans/sharelink/internal/douyin"
	"github.com/procmeans/sharelink/internal/httpapi"
	"github.com/procmeans/sharelink/internal/httpclient"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	ttwid := douyin.NewTTWidManager(httpclient.NewFollowing())
	if err := ttwid.Refresh(); err != nil {
		slog.Error("ttwid refresh failed at startup", "err", err.Error())
		os.Exit(1)
	}
	slog.Info("ttwid acquired", "len", len(ttwid.Cookie()))

	c := cache.New(1000, 30*time.Minute)
	svc := douyin.NewService(ttwid, c)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	handler := httpapi.NewHandler(svc.Extract)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	slog.Info("server starting", "addr", srv.Addr)
	err := srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
