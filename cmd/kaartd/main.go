// Command kaartd is Kaart's local server.
//
// It runs on the user's own machine and listens on loopback by default. There
// is no account system and no remote service: the database is a file, and the
// binary that reads it is this one.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/romonzaman/kaart/internal/api"
	"github.com/romonzaman/kaart/internal/clock"
	"github.com/romonzaman/kaart/internal/store/sqlite"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

const (
	shutdownGrace     = 10 * time.Second
	readHeaderTimeout = 10 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
)

// originList collects a repeatable --cors-origin flag.
type originList []string

func (o *originList) String() string { return strings.Join(*o, ",") }

func (o *originList) Set(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return errors.New("cors origin must not be empty")
	}
	*o = append(*o, v)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "kaartd: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		dbPath        = flag.String("db", "./kaart.db", "path to the SQLite database file")
		addr          = flag.String("addr", "127.0.0.1:8080", "address to listen on")
		logLevel      = flag.String("log-level", "info", "log level: debug, info, warn, error")
		migrateOnly   = flag.Bool("migrate-only", false, "apply migrations and exit")
		showVersion   = flag.Bool("version", false, "print the version and exit")
		corsOrigins   originList
		defaultOrigin = "http://localhost:8081" // Expo web's default dev server
	)
	flag.Var(&corsOrigins, "cors-origin",
		"browser origin allowed to call the API; repeatable (default "+defaultOrigin+")")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}
	if len(corsOrigins) == 0 {
		corsOrigins = originList{defaultOrigin}
	}

	level, err := parseLevel(*logLevel)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			logger.Error("closing database", slog.String("error", err.Error()))
		}
	}()

	if *migrateOnly {
		logger.Info("migrations applied", slog.String("db", *dbPath))
		return nil
	}

	srv := &http.Server{
		Addr: *addr,
		Handler: api.New(api.Config{
			Store:       st,
			Clock:       clock.New(),
			Logger:      logger,
			CORSOrigins: corsOrigins,
			Version:     version,
		}),
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("kaart is listening",
			slog.String("addr", *addr),
			slog.String("db", *dbPath),
			slog.String("version", version),
			slog.Any("cors_origins", []string(corsOrigins)),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listening on %s: %w", *addr, err)
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down", slog.Duration("grace", shutdownGrace))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down: %w", err)
	}
	if err := <-errCh; err != nil {
		return err
	}

	logger.Info("goodbye")
	return nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q: use debug, info, warn, or error", s)
	}
}
