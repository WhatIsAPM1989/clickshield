package store

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Store struct {
	Pool          *pgxpool.Pool
	sessionSecret []byte
}

func New(ctx context.Context, url string, sessionSecret []byte) (*Store, error) {
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
	if len(sessionSecret) < 32 {
		pool.Close()
		return nil, errors.New("session secret must contain at least 32 bytes")
	}
	secretCopy := append([]byte(nil), sessionSecret...)
	return &Store{Pool: pool, sessionSecret: secretCopy}, nil
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

func UUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	out := make([]byte, 36)
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:])
	return string(out), nil
}
func Token(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func PublicKey() (string, error) { t, e := Token(18); return "pk_" + t, e }
func ValidUUID(v string) bool {
	if len(v) != 36 {
		return false
	}
	for i, r := range v {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}
