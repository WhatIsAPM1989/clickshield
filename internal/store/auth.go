package store

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Admin struct {
	UserID, Username, CSRF string
	ExpiresAt              time.Time
}

func (s *Store) EnsureAdmin(ctx context.Context, u, p string) error {
	var id, hash string
	e := s.Pool.QueryRow(ctx, `SELECT id::text,password_hash FROM admin_users WHERE lower(username)=lower($1)`, u).Scan(&id, &hash)
	if e == nil {
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil {
			_, e = s.Pool.Exec(ctx, `UPDATE admin_users SET username=$2,active=true,updated_at=now() WHERE id=$1`, id, u)
			return e
		}
		h, e := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
		if e != nil {
			return e
		}
		// A new password ends every session opened with the old one.
		tx, e := s.Pool.Begin(ctx)
		if e != nil {
			return e
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, e = tx.Exec(ctx, `UPDATE admin_users SET username=$2,password_hash=$3,active=true,updated_at=now() WHERE id=$1`, id, u, string(h)); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `DELETE FROM admin_sessions WHERE admin_user_id=$1`, id); e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	if !IsNotFound(e) {
		return e
	}
	id, e = UUID()
	if e != nil {
		return e
	}
	h, e := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	_, e = s.Pool.Exec(ctx, `INSERT INTO admin_users(id,username,password_hash) VALUES($1,$2,$3)`, id, u, string(h))
	return e
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("clickshield-unknown-user"), bcrypt.DefaultCost)

func (s *Store) Authenticate(ctx context.Context, u, p string) (Admin, error) {
	var a Admin
	var hash string
	var active bool
	e := s.Pool.QueryRow(ctx, `SELECT id::text,username,password_hash,active FROM admin_users WHERE lower(username)=lower($1)`, strings.TrimSpace(u)).Scan(&a.UserID, &a.Username, &hash, &active)
	if e != nil {
		// Spend the same bcrypt time for an unknown user, so the response
		// time does not reveal which user names exist.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(p))
		return Admin{}, fmt.Errorf("invalid credentials")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) != nil || !active {
		return Admin{}, fmt.Errorf("invalid credentials")
	}
	return a, nil
}
func (s *Store) CreateAdminSession(ctx context.Context, userID string, ttl time.Duration) (string, Admin, error) {
	sid, e := UUID()
	if e != nil {
		return "", Admin{}, e
	}
	raw, e := Token(32)
	if e != nil {
		return "", Admin{}, e
	}
	csrf, e := Token(24)
	if e != nil {
		return "", Admin{}, e
	}
	h := s.hashAdminToken(raw)
	exp := time.Now().UTC().Add(ttl)
	_, e = s.Pool.Exec(ctx, `INSERT INTO admin_sessions(id,admin_user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,$5)`, sid, userID, h, csrf, exp)
	if e != nil {
		return "", Admin{}, e
	}
	var a Admin
	a.UserID = userID
	a.CSRF = csrf
	a.ExpiresAt = exp
	e = s.Pool.QueryRow(ctx, `SELECT username FROM admin_users WHERE id=$1`, userID).Scan(&a.Username)
	return raw, a, e
}
func (s *Store) AdminSession(ctx context.Context, raw string) (Admin, error) {
	h := s.hashAdminToken(raw)
	var a Admin
	e := s.Pool.QueryRow(ctx, `SELECT u.id::text,u.username,s.csrf_token,s.expires_at FROM admin_sessions s JOIN admin_users u ON u.id=s.admin_user_id WHERE s.token_hash=$1 AND s.expires_at>now() AND u.active`, h).Scan(&a.UserID, &a.Username, &a.CSRF, &a.ExpiresAt)
	if e == nil {
		_, _ = s.Pool.Exec(ctx, `UPDATE admin_sessions SET last_seen_at=now() WHERE token_hash=$1`, h)
	}
	return a, e
}
func (s *Store) DeleteAdminSession(ctx context.Context, raw string) error {
	h := s.hashAdminToken(raw)
	_, e := s.Pool.Exec(ctx, `DELETE FROM admin_sessions WHERE token_hash=$1`, h)
	return e
}
func (s *Store) hashAdminToken(raw string) []byte {
	mac := hmac.New(sha256.New, s.sessionSecret)
	_, _ = mac.Write([]byte(raw))
	return mac.Sum(nil)
}

func (s *Store) Audit(ctx context.Context, user, action, typ, id string, detail any) {
	b, e := json.Marshal(detail)
	if e != nil {
		b = []byte(`{}`)
	}
	_, _ = s.Pool.Exec(ctx, `INSERT INTO audit_log(admin_user_id,action,entity_type,entity_id,details) VALUES(NULLIF($1,'')::uuid,$2,$3,$4,$5::jsonb)`, user, action, typ, id, b)
}
