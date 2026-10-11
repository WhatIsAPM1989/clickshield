package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lamooof/clickshield/internal/domain"
)

func scanProject(row interface{ Scan(...any) error }) (domain.Project, error) {
	var p domain.Project
	var cfg []byte
	err := row.Scan(&p.ID, &p.Name, &p.PublicKey, &p.AllowedOrigins, &p.Mode, &cfg, &p.CreatedAt, &p.ReplayEnabled, &p.ReplaySampleRate, &p.ReplayRetentionDays)
	p.ScoringConfig = cfg
	return p, err
}
func (s *Store) ProjectByKey(ctx context.Context, key string) (domain.Project, error) {
	return scanProject(s.Pool.QueryRow(ctx, `SELECT id::text,name,public_key,allowed_origins,mode,scoring_config,created_at,replay_enabled,replay_sample_rate,replay_retention_days FROM projects WHERE public_key=$1`, key))
}
func (s *Store) Project(ctx context.Context, id string) (domain.Project, error) {
	return scanProject(s.Pool.QueryRow(ctx, `SELECT id::text,name,public_key,allowed_origins,mode,scoring_config,created_at,replay_enabled,replay_sample_rate,replay_retention_days FROM projects WHERE id=$1`, id))
}
func (s *Store) Projects(ctx context.Context) ([]domain.Project, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text,name,public_key,allowed_origins,mode,scoring_config,created_at,replay_enabled,replay_sample_rate,replay_retention_days FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		p, e := scanProject(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) EnsureProject(ctx context.Context, name, key string, origins []string, scoring any) (domain.Project, error) {
	if strings.TrimSpace(key) == "" {
		var e error
		key, e = PublicKey()
		if e != nil {
			return domain.Project{}, e
		}
	}
	id, e := UUID()
	if e != nil {
		return domain.Project{}, e
	}
	cfg, e := json.Marshal(scoring)
	if e != nil {
		return domain.Project{}, e
	}
	_, e = s.Pool.Exec(ctx, `INSERT INTO projects(id,name,public_key,allowed_origins,scoring_config) VALUES($1,$2,$3,$4,$5::jsonb) ON CONFLICT(public_key) DO NOTHING`, id, name, key, origins, cfg)
	if e != nil {
		return domain.Project{}, e
	}
	return s.ProjectByKey(ctx, key)
}
func (s *Store) CreateProject(ctx context.Context, name string, origins []string, mode string, cfg []byte) (domain.Project, error) {
	if !json.Valid(cfg) {
		return domain.Project{}, fmt.Errorf("invalid scoring config")
	}
	id, e := UUID()
	if e != nil {
		return domain.Project{}, e
	}
	key, e := PublicKey()
	if e != nil {
		return domain.Project{}, e
	}
	mode = domain.NormalizeMode(mode)
	_, e = s.Pool.Exec(ctx, `INSERT INTO projects(id,name,public_key,allowed_origins,mode,scoring_config) VALUES($1,$2,$3,$4,$5,$6::jsonb)`, id, name, key, origins, mode, cfg)
	if e != nil {
		return domain.Project{}, e
	}
	return s.Project(ctx, id)
}
func (s *Store) UpdateProject(ctx context.Context, id, name string, origins []string, mode string, cfg []byte) error {
	if !json.Valid(cfg) {
		return fmt.Errorf("invalid scoring config")
	}
	mode = domain.NormalizeMode(mode)
	tag, e := s.Pool.Exec(ctx, `UPDATE projects SET name=$2,allowed_origins=$3,mode=$4,scoring_config=$5::jsonb,updated_at=now() WHERE id=$1`, id, name, origins, mode, cfg)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}
