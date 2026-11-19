package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"businessautomation/internal/app"
	"businessautomation/internal/core"
	"businessautomation/internal/repository"
	"businessautomation/internal/telemetry"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) == 2 && os.Args[1] == "import-json" {
		return importJSON()
	}
	if len(os.Args) != 1 {
		return errors.New("usage: api [import-json]")
	}
	metricsToken := os.Getenv("METRICS_TOKEN")
	if metricsToken != "" && len(metricsToken) < 32 {
		return errors.New("METRICS_TOKEN must be at least 32 bytes")
	}
	path := os.Getenv("DATA_FILE")
	if path == "" {
		path = "data/platform.json"
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var repo core.Repository
	var pingDatabase func(context.Context) error
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		startup, cancel := context.WithTimeout(ctx, 30*time.Second)
		database, err := repository.OpenPostgres(startup, dsn)
		cancel()
		if err != nil {
			return err
		}
		defer database.Close()
		repo = database
		pingDatabase = database.Ping
	} else {
		file, err := repository.Open(path)
		if err != nil {
			return err
		}
		repo = file
	}
	mux := http.NewServeMux()
	mux.Handle("/", app.NewHandlerWithContext(ctx, repo))
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if pingDatabase != nil {
			probe, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			if err := pingDatabase(probe); err != nil {
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := telemetry.Handler(mux, metricsToken)
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { slog.Info("API listening", "address", addr); done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func importJSON() error {
	dsn, path := os.Getenv("DATABASE_URL"), os.Getenv("DATA_FILE")
	if dsn == "" || path == "" {
		return errors.New("import-json requires DATABASE_URL and DATA_FILE")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	database, err := repository.OpenPostgres(ctx, dsn)
	if err != nil {
		return err
	}
	defer database.Close()
	counts, err := database.ImportFile(ctx, path)
	if err != nil {
		return err
	}
	slog.Info("JSON import committed", "users", counts.Users, "sessions", counts.Sessions, "records", counts.Records)
	return nil
}
