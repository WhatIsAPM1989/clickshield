package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInviteInvalid = errors.New("invite is invalid or expired")
	ErrUserExists    = errors.New("user already exists")
)

type Invite struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) CreateInvite(ctx context.Context, by string, ttl time.Duration) (string, time.Time, error) {
	id, e := UUID()
	if e != nil {
		return "", time.Time{}, e
	}
	raw, e := Token(32)
	if e != nil {
		return "", time.Time{}, e
	}
	exp := time.Now().UTC().Add(ttl)
	_, e = s.Pool.Exec(ctx, `INSERT INTO admin_invites(id,token_hash,created_by,expires_at) VALUES($1,$2,NULLIF($3,'')::uuid,$4)`, id, s.hashAdminToken(raw), by, exp)
	return raw, exp, e
}

func (s *Store) ListInvites(ctx context.Context) ([]Invite, error) {
	rows, e := s.Pool.Query(ctx, `SELECT id::text,created_at,expires_at FROM admin_invites WHERE used_at IS NULL AND expires_at>now() ORDER BY created_at DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Invite{}
	for rows.Next() {
		var i Invite
		if e = rows.Scan(&i.ID, &i.CreatedAt, &i.ExpiresAt); e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Store) DeleteInvite(ctx context.Context, id string) error {
	_, e := s.Pool.Exec(ctx, `DELETE FROM admin_invites WHERE id=$1 AND used_at IS NULL`, id)
	return e
}

func (s *Store) InviteValid(ctx context.Context, raw string) bool {
	var ok bool
	_ = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_invites WHERE token_hash=$1 AND used_at IS NULL AND expires_at>now())`, s.hashAdminToken(raw)).Scan(&ok)
	return ok
}

// AcceptInvite burns the invite and creates the user in one transaction,
// so a link can never open more than one account.
func (s *Store) AcceptInvite(ctx context.Context, raw, email, password string) (string, error) {
	h, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return "", e
	}
	uid, e := UUID()
	if e != nil {
		return "", e
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var inviteID string
	e = tx.QueryRow(ctx, `UPDATE admin_invites SET used_at=now() WHERE token_hash=$1 AND used_at IS NULL AND expires_at>now() RETURNING id::text`, s.hashAdminToken(raw)).Scan(&inviteID)
	if IsNotFound(e) {
		return "", ErrInviteInvalid
	}
	if e != nil {
		return "", e
	}
	var pg *pgconn.PgError
	_, e = tx.Exec(ctx, `INSERT INTO admin_users(id,username,password_hash) VALUES($1,$2,$3)`, uid, strings.ToLower(strings.TrimSpace(email)), string(h))
	if errors.As(e, &pg) && pg.Code == "23505" {
		return "", ErrUserExists
	}
	if e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, `UPDATE admin_invites SET used_by=$2 WHERE id=$1`, inviteID, uid); e != nil {
		return "", e
	}
	return uid, tx.Commit(ctx)
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, e := s.Pool.Query(ctx, `SELECT id::text,username,active,created_at FROM admin_users ORDER BY created_at`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if e = rows.Scan(&u.ID, &u.Username, &u.Active, &u.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UserName(ctx context.Context, id string) (string, error) {
	var name string
	e := s.Pool.QueryRow(ctx, `SELECT username FROM admin_users WHERE id=$1`, id).Scan(&name)
	return name, e
}

// SetUserActive disables or re-enables a user; disabling ends all their sessions.
func (s *Store) SetUserActive(ctx context.Context, id string, active bool) error {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, e = tx.Exec(ctx, `UPDATE admin_users SET active=$2,updated_at=now() WHERE id=$1`, id, active); e != nil {
		return e
	}
	if !active {
		if _, e = tx.Exec(ctx, `DELETE FROM admin_sessions WHERE admin_user_id=$1`, id); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
