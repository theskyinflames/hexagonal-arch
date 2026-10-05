package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/acme/library/internal/lending/app"
	"github.com/acme/library/internal/lending/infra/httpapi"
	"github.com/acme/library/internal/lending/infra/membersapi"
	"github.com/acme/library/internal/lending/infra/postgres"
)

func main() {
	if err := run(); err != nil {
		slog.Error("library stopped", "error", err)
		os.Exit(1)
	}
}

// config is read here and nowhere else; adapters receive plain values.
type config struct {
	addr       string
	dbURL      string
	membersURL string
}

func loadConfig() (config, error) {
	c := config{
		addr:       os.Getenv("ADDR"),
		dbURL:      os.Getenv("DATABASE_URL"),
		membersURL: os.Getenv("MEMBERS_URL"),
	}
	if c.addr == "" {
		c.addr = ":8080"
	}
	if c.dbURL == "" || c.membersURL == "" {
		return c, errors.New("DATABASE_URL and MEMBERS_URL are required")
	}
	return c, nil
}

// run is the composition root: the only place that knows the concrete
// adapters. It builds driven adapters, injects them into use cases and
// hands the use cases to the driving adapters.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", cfg.dbURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// Driven adapters.
	books := postgres.Books{DB: db}
	members := membersapi.Client{BaseURL: cfg.membersURL, HTTP: &http.Client{Timeout: 3 * time.Second}}

	// Use cases.
	uc := httpapi.UseCases{
		AddBook:  app.AddBookHandler{Books: books},
		LendBook: app.LendBookHandler{Books: books, Members: members},
		GetBook:  app.GetBookHandler{Books: books},
	}

	// Driving adapters.
	mux := http.NewServeMux()
	httpapi.Routes(mux, uc, log)
	srv := &http.Server{Addr: cfg.addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Info("listening", "addr", cfg.addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
