package analytics

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lamooof/clickshield/internal/clientmeta"
	"github.com/lamooof/clickshield/internal/ident"
)

var ErrNotFound = errors.New("not found")

type Repository struct{ Pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool} }

func Open(ctx context.Context, dsn string) (*Repository, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns, cfg.MinConns = 4, 0
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "20000"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return NewRepository(pool), nil
}

func (r *Repository) Close()                    { r.Pool.Close() }
func IsNotFound(err error) bool                 { return errors.Is(err, pgx.ErrNoRows) }
func UUID() (string, error)                     { return ident.UUID() }
func Token(n int) (string, error)               { return ident.Token(n) }
func ValidUUID(v string) bool                   { return ident.ValidUUID(v) }
func ParseUserAgent(ua string) (string, string) { return clientmeta.ParseUserAgent(ua) }
