package analytics

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type AnalyticsAPIKey struct {
	ID         string     `json:"id"`
	ProjectID  string     `json:"project_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func (s *Repository) CreateAnalyticsKey(ctx context.Context, pid, name string, days int) (AnalyticsAPIKey, string, error) {
	var k AnalyticsAPIKey
	if !ValidUUID(pid) || strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 120 || days < 1 || days > 365 {
		return k, "", errors.New("project, name and lifetime 1–365 days required")
	}
	token, e := Token(32)
	if e != nil {
		return k, "", e
	}
	token = "csak_" + token
	id, e := UUID()
	if e != nil {
		return k, "", e
	}
	hash := sha256.Sum256([]byte(token))
	k = AnalyticsAPIKey{ID: id, ProjectID: pid, Name: name, Prefix: token[:13], CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)}
	_, e = s.Pool.Exec(ctx, `INSERT INTO analytics.analytics_api_keys(id,project_id,name,token_hash,prefix,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, id, pid, name, hash[:], k.Prefix, k.ExpiresAt)
	return k, token, e
}
func (s *Repository) AnalyticsKey(ctx context.Context, token string) (string, error) {
	hash := sha256.Sum256([]byte(token))
	var pid string
	e := s.Pool.QueryRow(ctx, `UPDATE analytics.analytics_api_keys SET last_used_at=now() WHERE token_hash=$1 AND revoked_at IS NULL AND expires_at>now() RETURNING project_id::text`, hash[:]).Scan(&pid)
	return pid, e
}
func (s *Repository) AnalyticsKeys(ctx context.Context, pid string) ([]AnalyticsAPIKey, error) {
	rows, e := s.Pool.Query(ctx, `SELECT id::text,project_id::text,name,prefix,expires_at,created_at,revoked_at,last_used_at FROM analytics.analytics_api_keys WHERE project_id=$1 ORDER BY created_at DESC`, pid)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []AnalyticsAPIKey{}
	for rows.Next() {
		var k AnalyticsAPIKey
		if e = rows.Scan(&k.ID, &k.ProjectID, &k.Name, &k.Prefix, &k.ExpiresAt, &k.CreatedAt, &k.RevokedAt, &k.LastUsedAt); e != nil {
			return nil, e
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
func (s *Repository) RevokeAnalyticsKey(ctx context.Context, pid, id string) error {
	tag, e := s.Pool.Exec(ctx, `UPDATE analytics.analytics_api_keys SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1 AND project_id=$2`, id, pid)
	if e == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return e
}

type TrackedLink struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Destination string `json:"destination"`
	Enabled     bool   `json:"enabled"`
	Hits        int64  `json:"hits"`
}
type TrackingPixel struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Hits      int64  `json:"hits"`
}
type Annotation struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	At        time.Time `json:"at"`
	Text      string    `json:"text"`
}

var linkSlug = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,80}$`)

func (s *Repository) SaveLink(ctx context.Context, l TrackedLink) (TrackedLink, error) {
	u, e := url.Parse(l.Destination)
	if e != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || len(l.Destination) > 4096 || !ValidUUID(l.ProjectID) || strings.TrimSpace(l.Name) == "" || utf8.RuneCountInString(l.Name) > 120 {
		return l, errors.New("valid project, name and HTTP(S) destination required")
	}
	if l.Slug == "" {
		v, e := Token(9)
		if e != nil {
			return l, e
		}
		l.Slug = v
	}
	if !linkSlug.MatchString(l.Slug) {
		return l, errors.New("slug requires 3–80 letters, digits, underscore or hyphen")
	}
	if l.ID == "" {
		l.ID, e = UUID()
		if e != nil {
			return l, e
		}
		_, e = s.Pool.Exec(ctx, `INSERT INTO analytics.tracked_links(id,project_id,slug,name,destination,enabled) VALUES($1,$2,$3,$4,$5,$6)`, l.ID, l.ProjectID, l.Slug, l.Name, l.Destination, l.Enabled)
	} else {
		tag, err := s.Pool.Exec(ctx, `UPDATE analytics.tracked_links SET slug=$3,name=$4,destination=$5,enabled=$6 WHERE id=$1 AND project_id=$2`, l.ID, l.ProjectID, l.Slug, l.Name, l.Destination, l.Enabled)
		e = err
		if e == nil && tag.RowsAffected() == 0 {
			e = ErrNotFound
		}
	}
	return l, e
}
func (s *Repository) SavePixel(ctx context.Context, p TrackingPixel) (TrackingPixel, error) {
	if !ValidUUID(p.ProjectID) || strings.TrimSpace(p.Name) == "" || utf8.RuneCountInString(p.Name) > 120 {
		return p, errors.New("project and name up to 120 characters required")
	}
	var e error
	if p.ID == "" {
		p.ID, e = UUID()
		if e != nil {
			return p, e
		}
		_, e = s.Pool.Exec(ctx, `INSERT INTO analytics.tracking_pixels(id,project_id,name,enabled) VALUES($1,$2,$3,$4)`, p.ID, p.ProjectID, p.Name, p.Enabled)
	} else {
		tag, err := s.Pool.Exec(ctx, `UPDATE analytics.tracking_pixels SET name=$3,enabled=$4 WHERE id=$1 AND project_id=$2`, p.ID, p.ProjectID, p.Name, p.Enabled)
		e = err
		if e == nil && tag.RowsAffected() == 0 {
			e = ErrNotFound
		}
	}
	return p, e
}
func (s *Repository) Links(ctx context.Context, pid string, from, to *time.Time) ([]TrackedLink, error) {
	rows, e := s.Pool.Query(ctx, `SELECT l.id::text,l.project_id::text,l.slug,l.name,l.destination,l.enabled,count(h.id) FROM analytics.tracked_links l LEFT JOIN analytics.tracking_hits h ON h.link_id=l.id AND ($2::timestamptz IS NULL OR h.occurred_at>=$2) AND ($3::timestamptz IS NULL OR h.occurred_at<$3) WHERE l.project_id=$1 GROUP BY l.id ORDER BY l.created_at,l.id`, pid, from, to)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []TrackedLink{}
	for rows.Next() {
		var l TrackedLink
		if e = rows.Scan(&l.ID, &l.ProjectID, &l.Slug, &l.Name, &l.Destination, &l.Enabled, &l.Hits); e != nil {
			return nil, e
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
func (s *Repository) Pixels(ctx context.Context, pid string, from, to *time.Time) ([]TrackingPixel, error) {
	rows, e := s.Pool.Query(ctx, `SELECT p.id::text,p.project_id::text,p.name,p.enabled,count(h.id) FROM analytics.tracking_pixels p LEFT JOIN analytics.tracking_hits h ON h.pixel_id=p.id AND ($2::timestamptz IS NULL OR h.occurred_at>=$2) AND ($3::timestamptz IS NULL OR h.occurred_at<$3) WHERE p.project_id=$1 GROUP BY p.id ORDER BY p.created_at,p.id`, pid, from, to)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []TrackingPixel{}
	for rows.Next() {
		var p TrackingPixel
		if e = rows.Scan(&p.ID, &p.ProjectID, &p.Name, &p.Enabled, &p.Hits); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Repository) HitLink(ctx context.Context, slug, ua, referrer, country string, record bool) (string, error) {
	var l TrackedLink
	e := s.Pool.QueryRow(ctx, `SELECT id::text,project_id::text,destination FROM analytics.tracked_links WHERE slug=$1 AND enabled`, slug).Scan(&l.ID, &l.ProjectID, &l.Destination)
	if e != nil {
		return "", e
	}
	if !record {
		return l.Destination, nil
	}
	browser, osName := ParseUserAgent(ua)
	_, e = s.Pool.Exec(ctx, `INSERT INTO analytics.tracking_hits(project_id,link_id,browser,os,referrer,country) SELECT project_id,id,$2,$3,$4,$5 FROM analytics.tracked_links WHERE id=$1 AND enabled`, l.ID, browser, osName, referrer, country)
	return l.Destination, e
}
func (s *Repository) HitPixel(ctx context.Context, id, ua, referrer, country string) error {
	browser, osName := ParseUserAgent(ua)
	tag, e := s.Pool.Exec(ctx, `INSERT INTO analytics.tracking_hits(project_id,pixel_id,browser,os,referrer,country) SELECT project_id,id,$2,$3,$4,$5 FROM analytics.tracking_pixels WHERE id=$1 AND enabled`, id, browser, osName, referrer, country)
	if e == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return e
}
func (s *Repository) SaveAnnotation(ctx context.Context, a Annotation) (Annotation, error) {
	if !ValidUUID(a.ProjectID) || a.At.IsZero() || strings.TrimSpace(a.Text) == "" || utf8.RuneCountInString(a.Text) > 500 {
		return a, errors.New("project, timestamp and text up to 500 characters required")
	}
	var e error
	if a.ID == "" {
		a.ID, e = UUID()
		if e != nil {
			return a, e
		}
		_, e = s.Pool.Exec(ctx, `INSERT INTO analytics.analytics_annotations(id,project_id,at,text) VALUES($1,$2,$3,$4)`, a.ID, a.ProjectID, a.At, a.Text)
	} else {
		tag, err := s.Pool.Exec(ctx, `UPDATE analytics.analytics_annotations SET at=$3,text=$4 WHERE id=$1 AND project_id=$2`, a.ID, a.ProjectID, a.At, a.Text)
		e = err
		if e == nil && tag.RowsAffected() == 0 {
			e = ErrNotFound
		}
	}
	return a, e
}
func (s *Repository) Annotations(ctx context.Context, pid string, from, to *time.Time) ([]Annotation, error) {
	rows, e := s.Pool.Query(ctx, `SELECT id::text,project_id::text,at,text FROM analytics.analytics_annotations WHERE project_id=$1 AND ($2::timestamptz IS NULL OR at>=$2) AND ($3::timestamptz IS NULL OR at<$3) ORDER BY at,id`, pid, from, to)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Annotation{}
	for rows.Next() {
		var a Annotation
		if e = rows.Scan(&a.ID, &a.ProjectID, &a.At, &a.Text); e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Repository) DeleteAnalyticsTool(ctx context.Context, pid, id, kind string) error {
	tables := map[string]string{"links": "analytics.tracked_links", "pixels": "analytics.tracking_pixels", "annotations": "analytics.analytics_annotations"}
	table, ok := tables[kind]
	if !ok {
		return errors.New("unknown tool")
	}
	tag, e := s.Pool.Exec(ctx, `DELETE FROM `+table+` WHERE id=$1 AND project_id=$2`, id, pid)
	if e == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return e
}
