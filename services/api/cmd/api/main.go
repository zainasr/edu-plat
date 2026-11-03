// Command api is the HTTP entry point. It only wires dependencies together.
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

	"github.com/zainasr/edu-plat/services/api/internal/api"
	"github.com/zainasr/edu-plat/services/api/internal/api/gen"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	// Routes come from the generated OpenAPI handler; api.Server implements them.
	handler := gen.HandlerFromMux(gen.NewStrictHandler(api.NewServer(), nil), http.NewServeMux())

	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", "err", err)
	}
}
