package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/assessgap/assessgap/internal/store"
	"github.com/assessgap/assessgap/internal/web"
)

func runServe(ctx context.Context, env *Env, args []string) error {
	fl := newFlags(env, "serve", "[-addr host:port] [-db path] [-app path] [-dev=false]")
	addr := fl.String("addr", "127.0.0.1:8080", "listen address")
	var cookPath string
	fl.StringVar(&cookPath, "db", "", "county database, opened read-only (default $ASSESSGAP_DB, else "+DefaultDB+")")
	fl.StringVar(&cookPath, "cook", "", "same as -db")
	appPath := fl.String("app", "data/app.db", "app database (email signups), created if missing")
	dev := fl.Bool("dev", true, "dev mode (noindex)")
	if _, err := parseArgs(fl, args); err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(env.Stderr, nil))

	cook, err := openCounty(dbPath(env, cookPath))
	if err != nil {
		return err
	}
	defer cook.Close()
	app, err := store.OpenApp(*appPath)
	if err != nil {
		return fmt.Errorf("app database %s: %w", *appPath, err)
	}
	defer app.Close()
	site, err := web.New(cook, app, *dev, log)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr: *addr, Handler: site.Handler(),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second,
	}
	// ListenAndServe returns as soon as Shutdown starts, so wait for in-flight
	// requests to finish before the deferred database closes run.
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shut); err != nil {
			log.Error("shutdown", "err", err)
		}
	}()
	// Bind before logging "listening", so the line means the port is taken
	// (a bad -addr fails here instead of after a misleading log line).
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	log.Info("listening", "addr", ln.Addr().String(), "dev", *dev)
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-drained
	return nil
}
