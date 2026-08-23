// Package sqlite is the SQLite implementation of store.Store.
//
// It is the only implementation in v1. Everything outside this package talks to
// the store.Store interface, so a Postgres or Supabase backend can be added as
// a sibling package without touching handlers or the scheduler.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // pure-Go driver, registered as "sqlite"

	"github.com/romonzaman/kaart/internal/store"
	"github.com/romonzaman/kaart/internal/store/migrations"
)

// tsLayout is a fixed-width RFC3339 layout. Fixed width matters: SQLite compares
// these timestamps as text, so every value must have the same number of
// fractional digits for lexicographic order to equal chronological order.
// time.RFC3339Nano trims trailing zeros and would break that.
const tsLayout = "2006-01-02T15:04:05.000000000Z07:00"

// defaultCardLimit is used when CardFilter.Limit is zero.
const defaultCardLimit = 50

// Store is a SQLite-backed store.Store.
type Store struct {
	db *sql.DB
}

// compile-time check
var _ store.Store = (*Store)(nil)

// Open opens (creating if necessary) the SQLite database at path and runs all
// pending migrations. Pass ":memory:" only for throwaway use — the review log
// is the point of this application, so tests use temp files instead.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := dsnFor(path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database %q: %w", path, err)
	}

	// SQLite tolerates exactly one writer. Keeping a single connection avoids
	// SQLITE_BUSY entirely for a local single-user server.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connecting to sqlite database %q: %w", path, err)
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating database %q: %w", path, err)
	}

	return &Store{db: db}, nil
}

func dsnFor(path string) string {
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	return "file:" + path + "?" + q.Encode()
}

func migrate(db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("setting goose dialect: %w", err)
	}
	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}
	return nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("closing sqlite database: %w", err)
	}
	return nil
}

// DB exposes the underlying handle for diagnostics only. It must not be used to
// run application queries from outside this package.
func (s *Store) DB() *sql.DB { return s.db }

// withTx runs fn inside a transaction, rolling back on error or panic.
func (s *Store) withTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// --- value conversion helpers ---

// ts renders a time for storage: fixed-width RFC3339 in UTC.
func ts(t time.Time) string { return t.UTC().Format(tsLayout) }

// tsPtr renders a nullable time.
func tsPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return ts(*t)
}

// parseTS reads a stored timestamp back.
func parseTS(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing timestamp %q: %w", s, err)
	}
	return t.UTC(), nil
}

// parseTSNull reads a nullable stored timestamp.
func parseTSNull(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid {
		return nil, nil //nolint:nilnil // absence is the meaningful value here
	}
	t, err := parseTS(ns.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// encodeTags stores tags as a JSON array. A nil slice becomes "[]", never "null".
func encodeTags(tags []string) (string, error) {
	if tags == nil {
		tags = []string{}
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return "", fmt.Errorf("encoding tags: %w", err)
	}
	return string(b), nil
}

// decodeTags reads a stored JSON tag array, always returning a non-nil slice.
func decodeTags(raw string) ([]string, error) {
	if raw == "" {
		return []string{}, nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return nil, fmt.Errorf("decoding tags %q: %w", raw, err)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, nil
}

// encodeWeights stores FSRS weights as a JSON array, or NULL when unset.
func encodeWeights(w []float64) (any, error) {
	if len(w) == 0 {
		return nil, nil //nolint:nilnil // NULL is the meaningful value here
	}
	b, err := json.Marshal(w)
	if err != nil {
		return nil, fmt.Errorf("encoding fsrs weights: %w", err)
	}
	return string(b), nil
}

func decodeWeights(ns sql.NullString) ([]float64, error) {
	if !ns.Valid || ns.String == "" {
		return nil, nil
	}
	var w []float64
	if err := json.Unmarshal([]byte(ns.String), &w); err != nil {
		return nil, fmt.Errorf("decoding fsrs weights: %w", err)
	}
	return w, nil
}

// affected converts a zero-row-affected result into store.ErrNotFound.
func affected(res sql.Result, what string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected for %s: %w", what, err)
	}
	if n == 0 {
		return fmt.Errorf("%s: %w", what, store.ErrNotFound)
	}
	return nil
}

// notFound wraps sql.ErrNoRows as store.ErrNotFound and everything else verbatim.
func notFound(err error, what string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s: %w", what, store.ErrNotFound)
	}
	return fmt.Errorf("%s: %w", what, err)
}
