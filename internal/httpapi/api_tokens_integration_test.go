package httpapi

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/lamooof/clickshield/internal/access"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newID() string {
	id, _ := store.UUID()
	return id
}

func (env *apiEnv) session(t *testing.T, p domain.Project) string {
	t.Helper()
	sid := newID()
	batch := domain.EventBatch{SessionID: sid, VisitorID: newID(), BatchID: newID(), PageURL: "https://example.test/", PageIndex: 1, Events: []domain.Event{{Type: "pageview", TS: time.Now().UnixMilli(), PageIndex: 1}}}
	if _, err := env.st.Ingest(context.Background(), p, batch, domain.RemoteMeta{}); err != nil {
		t.Fatal(err)
	}
	return sid
}

func toolCall(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func TestAPITokenScopesAndProjects(t *testing.T) {
	env := newAPIEnv(t)
	p, other := env.p, env.other
	sid, otherSID := env.session(t, p), env.session(t, other)
	_, projectToken := env.token(t, p.ID, access.SessionsWrite)
	_, allToken := env.token(t, "", access.ProjectsRead)

	for _, c := range []struct {
		method, path, token string
		body                any
		want                int
	}{
		{"GET", "/v1/sessions", projectToken, nil, 200},
		{"GET", "/v1/sessions?project_id=" + other.ID, projectToken, nil, 403},
		{"GET", "/v1/sessions/" + sid, projectToken, nil, 200},
		{"GET", "/v1/sessions/" + otherSID, projectToken, nil, 404},
		{"POST", "/v1/sessions/" + sid + "/label", projectToken, map[string]string{"label": "HUMAN"}, 200}, // no CSRF for Bearer
		{"POST", "/v1/sessions/" + otherSID + "/label", projectToken, map[string]string{"label": "BOT"}, 404},
		{"GET", "/v1/sessions/" + sid + "/replay", projectToken, nil, 403},
		{"GET", "/v1/projects", projectToken, nil, 403},
		{"GET", "/v1/operations", allToken, nil, 200},
		{"GET", "/v1/projects/" + other.ID + "/integrations", allToken, nil, 200},
		{"PUT", "/v1/projects/" + other.ID + "/replay", allToken, map[string]any{"enabled": false, "sample_rate": 10, "retention_days": 7}, 403},
		{"GET", "/v1/sessions", allToken, nil, 403},
		{"GET", "/v1/api-tokens", allToken, nil, 401},
		{"POST", "/v1/api-tokens", allToken, map[string]any{"name": "x", "scopes": access.All}, 401},
		{"GET", "/v1/users", allToken, nil, 401},
		{"GET", "/v1/invites", allToken, nil, 401},
		{"GET", "/v1/me", allToken, nil, 401},
	} {
		if code, data := env.call(t, c.method, c.path, c.body, c.token, false); code != c.want {
			t.Errorf("%s %s: %d, want %d (%v)", c.method, c.path, code, c.want, data)
		}
	}
	if _, data := env.call(t, "GET", "/v1/sessions", nil, projectToken, false); len(data["sessions"].([]any)) != 1 {
		t.Errorf("project token saw other sessions: %v", data)
	}
	// A project token cannot use routes for the whole installation.
	_, projectRead := env.token(t, p.ID, access.ProjectsRead)
	if code, data := env.call(t, "GET", "/v1/projects", nil, projectRead, false); code != 200 || len(data["projects"].([]any)) != 1 {
		t.Errorf("project token projects: %d %v", code, data)
	}
	if code, _ := env.call(t, "GET", "/v1/operations", nil, projectRead, false); code != 403 {
		t.Errorf("project token read operations: %d", code)
	}
	if code, _ := env.call(t, "POST", "/v1/api-tokens", map[string]any{"name": "x", "scopes": []string{"users:write"}}, "", true); code != 422 {
		t.Errorf("unknown scope accepted: %d", code)
	}

	// MCP lists only the tools the token may use.
	session := env.mcpSession(t, projectToken)
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	for _, name := range []string{"whoami", "list_sessions", "get_session", "label_session", "set_lead_outcome"} {
		if !slices.Contains(names, name) {
			t.Errorf("missing tool %s in %v", name, names)
		}
	}
	for _, name := range []string{"analytics_report", "get_session_replay", "list_projects", "create_project"} {
		if slices.Contains(names, name) {
			t.Errorf("tool %s offered without scope", name)
		}
	}
	if res := toolCall(t, session, "whoami", nil); res.IsError || res.StructuredContent.(map[string]any)["data"].(map[string]any)["project_id"] != p.ID {
		t.Errorf("whoami: %+v", res.StructuredContent)
	}
	if res := toolCall(t, session, "list_sessions", map[string]any{"classification": "WRONG"}); !res.IsError {
		t.Error("invalid filter accepted")
	}
	if res := toolCall(t, session, "get_session", map[string]any{"session_id": otherSID}); !res.IsError {
		t.Error("MCP read a session of another project")
	}
	if res := toolCall(t, session, "label_session", map[string]any{"session_id": sid, "label": "BOT", "note": "MCP"}); res.IsError {
		t.Errorf("label_session: %+v", res.Content)
	}
	if _, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_project", Arguments: map[string]any{}}); err == nil {
		t.Error("unknown tool accepted")
	}
	all := env.mcpSession(t, allToken)
	if res := toolCall(t, all, "list_integrations", nil); !res.IsError {
		t.Error("all-projects token used a project tool without project_id")
	}
	if res := toolCall(t, all, "get_operations", nil); res.IsError {
		t.Errorf("get_operations: %+v", res.Content)
	}

	// Tokens of a disabled user stop working.
	if _, err = env.st.Pool.Exec(context.Background(), `UPDATE admin_users SET active=false WHERE id=$1`, env.userID); err != nil {
		t.Fatal(err)
	}
	if code, _ := env.call(t, "GET", "/v1/operations", nil, allToken, false); code != 401 {
		t.Errorf("token of a disabled user: %d", code)
	}
}
