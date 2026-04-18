package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/sekolahpintar/dashboard-engine/internal/config"
	"github.com/sekolahpintar/dashboard-engine/internal/db"
	"github.com/sekolahpintar/dashboard-engine/internal/handler"
	"github.com/sekolahpintar/dashboard-engine/internal/middleware"
	"github.com/sekolahpintar/dashboard-engine/internal/repository"
	"github.com/sekolahpintar/dashboard-engine/internal/service"
)

func main() {
	// Load .env (ignored in production – env vars injected by Docker/compose)
	_ = godotenv.Load()

	cfg := config.Load()

	database, err := db.New(cfg)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	// Dependency injection
	repo := repository.NewDashboardRepo(database)
	svc := service.NewDashboardService(repo)
	h := handler.NewDashboardHandler(svc)

	// Router
	r := chi.NewRouter()
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"dashboard-engine"}`))
	})

	// All dashboard routes require JWT auth
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(cfg.JWTSecret, database))

		r.Route("/api/v1/dashboard", func(r chi.Router) {
			r.Get("/", h.Index)
			r.Get("/summary-cards", h.SummaryCards)
			r.Get("/financial-analytics", h.FinancialAnalytics)
			r.Get("/academic-attendance", h.AcademicAttendanceAnalytics)
			r.Get("/counseling-insights", h.CounselingInsights)
			r.Get("/ppdb-insights", h.PpdbInsights)
		})
	})

	srv := &http.Server{
		Addr:         ":" + cfg.AppPort,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	idleConnsClosed := make(chan struct{})
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("dashboard-engine: shutting down…")

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("dashboard-engine: graceful shutdown error: %v", err)
		}
		close(idleConnsClosed)
	}()

	log.Printf("dashboard-engine: listening on :%s", cfg.AppPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("dashboard-engine: %v", err)
	}
	<-idleConnsClosed
}
