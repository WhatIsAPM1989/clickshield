package store

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

func (s *Store) EnsureRawPartitions(ctx context.Context) error {
	s.partitionsMu.Lock()
	defer s.partitionsMu.Unlock()
	now := time.Now().UTC()
	if now.Before(s.partitionsUntil) {
		return nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(730163486)`); err != nil {
		return err
	}
	day := now.Truncate(24 * time.Hour)
	for i := 0; i <= 7; i++ {
		from := day.AddDate(0, 0, i)
		to := from.AddDate(0, 0, 1)
		name := pgx.Identifier{"raw_events_" + from.Format("20060102")}.Sanitize()
		query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s PARTITION OF raw_events FOR VALUES FROM ('%s') TO ('%s')", name, from.Format(time.RFC3339), to.Format(time.RFC3339))
		if _, err = tx.Exec(ctx, query); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	s.partitionsUntil = day.Add(24 * time.Hour)
	return nil
}

func (s *Store) DropRawPartitions(ctx context.Context, before time.Time) error {
	rows, err := s.Pool.Query(ctx, `SELECT c.relname FROM pg_inherits i JOIN pg_class c ON c.oid=i.inhrelid WHERE i.inhparent='raw_events'::regclass`)
	if err != nil {
		return err
	}
	var names []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, name := range names {
		day, err := time.Parse("20060102", strings.TrimPrefix(name, "raw_events_"))
		if err != nil || day.AddDate(0, 0, 1).After(before.UTC()) {
			continue
		}
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout='2s'`); err == nil {
			_, err = tx.Exec(ctx, "DROP TABLE IF EXISTS "+pgx.Identifier{name}.Sanitize())
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}
