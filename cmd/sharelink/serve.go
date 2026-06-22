package main

import (
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/procmeans/sharelink/internal/cache"
	"github.com/procmeans/sharelink/internal/douyin"
	"github.com/procmeans/sharelink/internal/extract"
	"github.com/procmeans/sharelink/internal/httpapi"
	"github.com/procmeans/sharelink/internal/httpclient"
	"github.com/procmeans/sharelink/internal/youtube"
)

func runServe(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	portFlag := fs.String("port", "", "listen port (defaults to $PORT or 8080)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	ttwid := douyin.NewTTWidManager(httpclient.NewNoRedirect())
	if err := ttwid.Refresh(); err != nil {
		slog.Error("ttwid refresh failed at startup", "err", err.Error())
		return 1
	}
	slog.Info("ttwid acquired", "len", len(ttwid.Cookie()))

	c := cache.New(1000, 30*time.Minute)
	router := extract.NewRouter(douyin.NewService(ttwid, c), youtube.NewService(youtube.Muxed))

	port := *portFlag
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      httpapi.NewHandler(router.Extract),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	slog.Info("server starting", "addr", srv.Addr)
	err := srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "err", err)
		return 1
	}
	return 0
}
