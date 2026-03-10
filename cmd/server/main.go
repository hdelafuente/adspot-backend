package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	chi "github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/adspot-backend/adspot-backend/internal/adspot"
	"github.com/adspot-backend/adspot-backend/internal/database"
	"github.com/adspot-backend/adspot-backend/internal/middleware"
)

func main() {
	// ── Database ──────────────────────────────────────────────────────────────
	db, err := database.Open("adspot.db")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(db, "migrations"); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// ── Router ────────────────────────────────────────────────────────────────
	r := chi.NewRouter()

	// Global middleware
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Logger)
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
		log.Printf("server listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	log.Println("server stopped")
}
