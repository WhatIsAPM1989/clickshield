package store

import (
	"context"
	"fmt"
)

// Serving checks schema compatibility; only the migrate command changes it.
func (s *Store) CheckSchema(ctx context.Context) error {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	var exists bool
	if err = s.Pool.QueryRow(ctx, `SELECT to_regclass('schema_migrations') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("database is not initialized; run clickshield migrate")
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	var missing int
	err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM unnest($1::text[]) AS required(name) WHERE NOT EXISTS(SELECT 1 FROM schema_migrations m WHERE m.name=required.name)`, names).Scan(&missing)
	if err == nil && missing != 0 {
		return fmt.Errorf("database requires %d migrations; run clickshield migrate", missing)
	}
	return err
}
