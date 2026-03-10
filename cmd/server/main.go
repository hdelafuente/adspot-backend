package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	chi "github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/adspot-backend/adspot-backend/internal/adspot"
	"github.com/adspot-backend/adspot-backend/internal/database"
	applogger "github.com/adspot-backend/adspot-backend/internal/logger"
	"github.com/adspot-backend/adspot-backend/internal/middleware"
)

func main() {
	// ── Logger ────────────────────────────────────────────────────────────────
	// LOG_LEVEL accepts: debug | info | warn | error  (default: info)
	l := applogger.New(os.Getenv("LOG_LEVEL"))
	slog.SetDefault(l)

	// ── Database ──────────────────────────────────────────────────────────────
	db, err := database.Open("adspot.db")
	if err != nil {
		slog.Error("open db", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := database.Migrate(db, "migrations"); err != nil {
		slog.Error("migrate", "error", err)
		os.Exit(1)
	}

	// ── Router ────────────────────────────────────────────────────────────────
	r := chi.NewRouter()

	// Global middleware (order matters: RequestID must run before Logger so the
	// request ID is available when the Logger middleware builds its log line).
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(middleware.Logger)      // structured JSON logger — replaces chiMiddleware.Logger
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.Timeout(5 * time.Second))
	r.Use(middleware.RateLimit(10))

	// Routes
	repo := adspot.NewRepository(db)
	r.Mount("/adspots", adspot.NewHandler(repo).Routes())

	// ── HTTP server ───────────────────────────────────────────────────────────
	addr := ":8080"
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen", "error", err)
			os.Exit(1)
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}
