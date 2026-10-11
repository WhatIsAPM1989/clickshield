package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lamooof/clickshield/internal/ident"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Store struct {
	Pool            *pgxpool.Pool
	sessionSecret   []byte
	partitionsMu    sync.Mutex
	partitionsUntil time.Time
}

func New(ctx context.Context, url string, sessionSecret []byte) (*Store, error) {
	if len(sessionSecret) < 32 {
		return nil, errors.New("session secret must contain at least 32 bytes")
	}
	st, err := NewDatabase(ctx, url)
	if err != nil {
		return nil, err
	}
	st.sessionSecret = append([]byte(nil), sessionSecret...)
	return st, nil
}

func NewDatabase(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns, cfg.MinConns = 20, 2
	cfg.MaxConnIdleTime, cfg.MaxConnLifetime = 5*time.Minute, time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	ping, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err = pool.Ping(ping); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{Pool: pool}, nil
}
func (s *Store) Close()         { s.Pool.Close() }
func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func (s *Store) Migrate(ctx context.Context) error {
	// A dedicated connection holds the migration lock across every transaction.
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(730163485)`); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(730163485)`)

	if _, err := s.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var exists bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		data, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(data)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func UUID() (string, error)       { return ident.UUID() }
func Token(n int) (string, error) { return ident.Token(n) }
func PublicKey() (string, error)  { return ident.PublicKey() }
func ValidUUID(v string) bool     { return ident.ValidUUID(v) }
