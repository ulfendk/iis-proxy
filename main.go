package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := LoadConfig(os.Getenv)
	if err != nil {
		logger.Error("configuration error", "err", err)
		os.Exit(1)
	}

	handler := NewProxyHandler(cfg, logger)
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// No blanket ReadTimeout/WriteTimeout: EWS request/response bodies
		// (large SOAP payloads, attachments) can legitimately take a
		// while. Per-phase timeouts on the upstream Transport (proxy.go)
		// bound the backend leg instead.
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("iis-proxy listening",
			"addr", cfg.ListenAddr,
			"upstream", cfg.UpstreamScheme+"://"+cfg.UpstreamHost,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	logger.Info("iis-proxy stopped")
}
