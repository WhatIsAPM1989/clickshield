package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/lamooof/clickshield/internal/access"
	"github.com/lamooof/clickshield/internal/analytics"
	"github.com/lamooof/clickshield/internal/domain"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lamooof/clickshield/internal/config"
	"github.com/lamooof/clickshield/internal/cryptox"
	"github.com/lamooof/clickshield/internal/scoring"
	"github.com/lamooof/clickshield/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type keyTransport struct {
	key  string
	next http.RoundTripper
}

func (t keyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.key)
	return t.next.RoundTrip(r)
}

// apiEnv is a server on a dedicated test database with two projects and an admin.
type apiEnv struct {
	st       *store.Store
	s        *Server
	srv      *httptest.Server
	p, other domain.Project
	cookie   string
	csrf     string
	userID   string
}

func newAPIEnv(t *testing.T) *apiEnv {
	t.Helper()
	dsn := os.Getenv("CLICKSHIELD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated PostgreSQL required")
	}
	ctx := context.Background()
	st, e := store.New(ctx, dsn, []byte("0123456789abcdef0123456789abcdef"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(st.Close)
	if e = st.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	env := &apiEnv{st: st}
	cfg, _ := json.Marshal(scoring.DefaultConfig())
	for _, dst := range []*domain.Project{&env.p, &env.other} {
		if *dst, e = st.CreateProject(ctx, "HTTP API test", []string{"https://example.test"}, "MONITORING", cfg); e != nil {
			t.Fatal(e)
		}
		id := dst.ID
		t.Cleanup(func() { st.Pool.Exec(ctx, `DELETE FROM projects WHERE id=$1`, id) })
	}
	username, _ := store.Token(9)
	if e = st.EnsureAdmin(ctx, username, "test-password-12345"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { st.Pool.Exec(ctx, `DELETE FROM admin_users WHERE username=$1`, username) })
	admin, e := st.Authenticate(ctx, username, "test-password-12345")
	if e != nil {
		t.Fatal(e)
	}
	cookie, a, e := st.CreateAdminSession(ctx, admin.UserID, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	env.cookie, env.csrf, env.userID = cookie, a.CSRF, admin.UserID
	cipher, _ := cryptox.New(bytes.Repeat([]byte("x"), 32))
	env.s = New(config.Config{MCPEnabled: true}, st, nil, cipher, slog.New(slog.NewTextHandler(io.Discard, nil)))
	env.srv = httptest.NewServer(env.s.Handler())
	t.Cleanup(env.srv.Close)
	return env
}

// call sends a request as the admin (cookie + CSRF) or with a Bearer token.
func (env *apiEnv) call(t *testing.T, method, path string, body any, token string, admin bool) (int, map[string]any) {
	t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	r, _ := http.NewRequest(method, env.srv.URL+path, bytes.NewReader(raw))
	if admin {
		r.AddCookie(&http.Cookie{Name: "clickshield_session", Value: env.cookie})
		r.Header.Set("X-CSRF-Token", env.csrf)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	res, e := http.DefaultClient.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	var data map[string]any
	json.NewDecoder(res.Body).Decode(&data)
	return res.StatusCode, data
}

// token issues an API token from the panel and returns its id and raw value.
func (env *apiEnv) token(t *testing.T, project string, scopes ...string) (string, string) {
	t.Helper()
	code, data := env.call(t, "POST", "/v1/api-tokens", map[string]any{"name": "test", "project_id": project, "scopes": scopes, "days": 1}, "", true)
	if code != 201 {
		t.Fatalf("create token: %d %+v", code, data)
	}
	return data["token"].(map[string]any)["id"].(string), data["raw"].(string)
}

// mcpSession connects the real SDK client with a Bearer token.
func (env *apiEnv) mcpSession(t *testing.T, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, e := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: env.srv.URL + "/mcp", HTTPClient: &http.Client{Transport: keyTransport{token, http.DefaultTransport}}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func TestAnalyticsAccessAndCampaignTools(t *testing.T) {
	env := newAPIEnv(t)
	ctx := context.Background()
	st, p, other := env.st, env.p, env.other
	call := func(method, path string, body any, key string, admin bool) (int, map[string]any) {
		t.Helper()
		return env.call(t, method, path, body, key, admin)
	}
	keyID, token := env.token(t, p.ID, access.AnalyticsRead)
	if code, _ := call("GET", "/v1/reports?report=sources", nil, token, false); code != 200 {
		t.Fatalf("scoped default: %d", code)
	}
	for _, path := range []string{"/v1/reports?project_id=" + other.ID, "/v1/reports?project_id=" + p.ID + "&project_id=" + other.ID, "/v1/projects/" + other.ID + "/definitions"} {
		if code, _ := call("GET", path, nil, token, false); code != 403 {
			t.Fatalf("cross-project read %s: %d", path, code)
		}
	}
	if code, _ := call("GET", "/v1/sessions", nil, token, false); code != 403 {
		t.Fatalf("token without scope read sessions: %d", code)
	}
	if code, _ := call("POST", "/v1/projects/"+p.ID+"/definitions", map[string]any{}, token, false); code != 403 {
		t.Fatalf("read token can write: %d", code)
	}
	_, data := call("GET", "/v1/api-tokens", nil, "", true)
	encoded, _ := json.Marshal(data)
	if strings.Contains(string(encoded), token) || strings.Contains(string(encoded), "token_hash") {
		t.Fatal("token list leaks secrets")
	}
	// Expired credentials are rejected independently of revocation.
	expiredToken, expired, e := st.CreateAPIToken(ctx, "", "expired", p.ID, []string{access.AnalyticsRead}, 1)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = st.Pool.Exec(ctx, `UPDATE api_tokens SET expires_at=now()-interval '1 second' WHERE id=$1`, expiredToken.ID); e != nil {
		t.Fatal(e)
	}
	if code, _ := call("GET", "/v1/reports", nil, expired, false); code != 401 {
		t.Fatalf("expired token accepted: %d", code)
	}

	sid, _ := store.UUID()
	vid, _ := store.UUID()
	bid, _ := store.UUID()
	batch := domain.EventBatch{SessionID: sid, VisitorID: vid, BatchID: bid, PageURL: "https://example.test/", PageIndex: 1, Events: []domain.Event{{Type: "pageview", TS: time.Now().UnixMilli(), PageIndex: 1}, {Type: "business", TS: time.Now().UnixMilli(), Name: "checkout", Properties: map[string]any{"step": float64(1)}}}}
	if _, err := st.Ingest(ctx, p, batch, domain.RemoteMeta{}); err != nil {
		t.Fatal(err)
	}
	for {
		ok, err := analytics.NewProjector(st.Pool).ProjectOne(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
	}
	code, httpEvents := call("GET", "/v1/analytics/events", nil, token, false)
	if code != 200 {
		t.Fatal(code, httpEvents)
	}
	// Real SDK client negotiates MCP and calls a project-bound tool.
	session := env.mcpSession(t, token)
	tools, e := session.ListTools(ctx, nil)
	if e != nil || len(tools.Tools) != 5 {
		t.Fatalf("MCP tools: %+v %v", tools, e)
	}
	result, e := session.CallTool(ctx, &mcp.CallToolParams{Name: "analytics_report", Arguments: map[string]any{"report": "events"}})
	if e != nil || result.IsError {
		t.Fatalf("MCP call: %+v %v", result, e)
	}

	rawStructured, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var mcpEvents map[string]any
	if err = json.Unmarshal(rawStructured, &mcpEvents); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(httpEvents, mcpEvents) {
		t.Fatalf("HTTP/MCP differ: HTTP=%+v MCP=%+v", httpEvents, mcpEvents)
	}
	for _, path := range []string{"/v1/analytics/events?timezone=invalid", "/v1/analytics/events?from=2026-10-02T00:00:00Z&to=2026-10-01T00:00:00Z"} {
		if code, _ := call("GET", path, nil, token, false); code != 400 {
			t.Fatalf("invalid query accepted: %s %d", path, code)
		}
	}
	r, _ := http.NewRequest("POST", env.srv.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Origin", "https://attacker.test")
	res, e := http.DefaultClient.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("MCP bad origin: %d", res.StatusCode)
	}
	// Campaign tools preserve explicit query parameters in the configured destination.
	code, data = call("POST", "/v1/projects/"+p.ID+"/tools/links", map[string]any{"name": "Campaign", "destination": "https://example.test/?utm_source=mail", "enabled": true}, "", true)
	if code != 200 {
		t.Fatalf("create link: %d %+v", code, data)
	}
	link := data["data"].(map[string]any)
	slug := link["slug"].(string)
	redirectClient := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e = redirectClient.Get(env.srv.URL + "/l/" + slug)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 302 || res.Header.Get("Location") != "https://example.test/?utm_source=mail" {
		t.Fatalf("redirect: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	r, _ = http.NewRequest("HEAD", env.srv.URL+"/l/"+slug, nil)
	res, e = redirectClient.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	_, data = call("GET", "/v1/projects/"+p.ID+"/tools/links", nil, token, false)
	if data["data"].([]any)[0].(map[string]any)["hits"].(float64) != 1 {
		t.Fatalf("HEAD counted: %+v", data)
	}
	code, data = call("POST", "/v1/projects/"+p.ID+"/tools/pixels", map[string]any{"name": "Email", "enabled": true}, "", true)
	if code != 200 {
		t.Fatalf("create pixel: %d %+v", code, data)
	}
	pixel := data["data"].(map[string]any)["id"].(string)
	res, e = http.Get(env.srv.URL + "/p/" + pixel)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !bytes.HasPrefix(raw, []byte("GIF89a")) || !strings.Contains(res.Header.Get("Cache-Control"), "no-store") {
		t.Fatalf("pixel: %d %q", res.StatusCode, raw)
	}
	code, data = call("POST", "/v1/projects/"+p.ID+"/tools/annotations", map[string]any{"text": "Campaign launched", "at": time.Now().UTC()}, "", true)
	if code != 200 {
		t.Fatalf("annotation: %d %+v", code, data)
	}
	if code, _ = call("DELETE", "/v1/projects/"+other.ID+"/tools/links/"+link["id"].(string), nil, "", true); code != 404 {
		t.Fatalf("cross-project delete: %d", code)
	}
	if code, _ = call("DELETE", "/v1/api-tokens/"+keyID, nil, "", true); code != 200 {
		t.Fatalf("revoke: %d", code)
	}
	if code, _ = call("GET", "/v1/reports", nil, token, false); code != 401 {
		t.Fatalf("revoked key accepted: %d", code)
	}
}
