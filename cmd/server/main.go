// Command server is the AssessGap website.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/assessgap/assessgap/internal/store"
	"github.com/assessgap/assessgap/internal/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	cookPath := flag.String("cook", "data/cook.db", "county data (read-only)")
	appPath := flag.String("app", "data/app.db", "app database (email signups)")
	dev := flag.Bool("dev", true, "dev mode (noindex)")
	flag.Parse()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cook, err := store.OpenReadOnly(*cookPath)
	if err != nil {
		log.Error("open county db", "err", err)
		os.Exit(1)
	}
	app, err := store.OpenApp(*appPath)
	if err != nil {
		log.Error("open app db", "err", err)
		os.Exit(1)
	}
	site, err := web.New(cook, app, *dev, log)
	if err != nil {
		log.Error("init", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr: *addr, Handler: site.Handler(),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shut)
	}()
	log.Info("listening", "addr", *addr, "dev", *dev)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
	cook.Close()
	app.Close()
}
