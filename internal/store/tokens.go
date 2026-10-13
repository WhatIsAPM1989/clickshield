package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// APIToken is a Bearer token for the HTTP API and MCP. An empty ProjectID
// means the token covers every project.
type APIToken struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	ProjectID   string     `json:"project_id"`
	ProjectName string     `json:"project_name"`
	Scopes      []string   `json:"scopes"`
	Prefix      string     `json:"prefix"`
	CreatedBy   string     `json:"created_by"`
	CreatorID   string     `json:"-"`
	ExpiresAt   time.Time  `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
}

const tokenColumns = `t.id::text,t.name,COALESCE(t.project_id::text,''),COALESCE(p.name,''),t.scopes,t.prefix,COALESCE(u.username,''),COALESCE(t.created_by::text,''),t.expires_at,t.created_at,t.revoked_at,t.last_used_at FROM api_tokens t LEFT JOIN projects p ON p.id=t.project_id LEFT JOIN admin_users u ON u.id=t.created_by`

func scanToken(row pgx.Row) (APIToken, error) {
	var t APIToken
	e := row.Scan(&t.ID, &t.Name, &t.ProjectID, &t.ProjectName, &t.Scopes, &t.Prefix, &t.CreatedBy, &t.CreatorID, &t.ExpiresAt, &t.CreatedAt, &t.RevokedAt, &t.LastUsedAt)
	return t, e
}

// CreateAPIToken stores a SHA-256 hash; the raw token is returned only here.
// Scopes must already be validated by the caller.
func (s *Store) CreateAPIToken(ctx context.Context, by, name, projectID string, scopes []string, days int) (APIToken, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 120 || days < 1 || days > 365 || (projectID != "" && !ValidUUID(projectID)) {
		return APIToken{}, "", errors.New("name and lifetime 1–365 days required")
	}
	id, e := UUID()
	if e != nil {
		return APIToken{}, "", e
	}
	raw, e := Token(32)
	if e != nil {
		return APIToken{}, "", e
	}
	raw = "cst_" + raw
	hash := sha256.Sum256([]byte(raw))
	_, e = s.Pool.Exec(ctx, `INSERT INTO api_tokens(id,name,project_id,scopes,token_hash,prefix,created_by,expires_at) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,NULLIF($7,'')::uuid,now()+make_interval(days=>$8))`, id, name, projectID, scopes, hash[:], raw[:12], by, days)
	if e != nil {
		return APIToken{}, "", e
	}
	t, e := scanToken(s.Pool.QueryRow(ctx, `SELECT `+tokenColumns+` WHERE t.id=$1`, id))
	return t, raw, e
}

// APITokenByRaw returns an active token. Tokens of a disabled user stop working.
func (s *Store) APITokenByRaw(ctx context.Context, raw string) (APIToken, error) {
	hash := sha256.Sum256([]byte(raw))
	t, e := scanToken(s.Pool.QueryRow(ctx, `SELECT `+tokenColumns+` WHERE t.token_hash=$1 AND t.revoked_at IS NULL AND t.expires_at>now() AND (t.created_by IS NULL OR u.active)`, hash[:]))
	if e == nil && (t.LastUsedAt == nil || time.Since(*t.LastUsedAt) > time.Minute) {
		_, _ = s.Pool.Exec(ctx, `UPDATE api_tokens SET last_used_at=now() WHERE id=$1`, t.ID)
	}
	return t, e
}

func (s *Store) APITokens(ctx context.Context) ([]APIToken, error) {
	rows, e := s.Pool.Query(ctx, `SELECT `+tokenColumns+` ORDER BY t.revoked_at IS NOT NULL, t.created_at DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		t, e := scanToken(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) RevokeAPIToken(ctx context.Context, id string) error {
	tag, e := s.Pool.Exec(ctx, `UPDATE api_tokens SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1`, id)
	if e == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return e
}

// SessionProject returns the project of a session.
func (s *Store) SessionProject(ctx context.Context, sessionID string) (string, error) {
	var pid string
	e := s.Pool.QueryRow(ctx, `SELECT project_id::text FROM sessions WHERE id=$1`, sessionID).Scan(&pid)
	return pid, e
}
