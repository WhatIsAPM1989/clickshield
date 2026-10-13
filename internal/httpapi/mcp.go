package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/lamooof/clickshield/internal/access"
	"github.com/lamooof/clickshield/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpInstructions = `ClickShield: antifraud for ad traffic (bot detection with explained risk scores) and product analytics.
Start with whoami to see the token scopes and project, then list_projects. A project token fixes the project, so project_id may be omitted; an all-projects token needs project_id for project tools.
Dates are RFC3339 with a timezone; "from" is inclusive, "to" exclusive. Classifications: HUMAN, SUSPICIOUS, BOT, KNOWN_BOT, UNCLASSIFIED, NO_BOTS (everything except bots).
Write tools change live data: session labels and lead outcomes are sent to Yandex Metrika, GA4 and VK Ads; project mode BLOCKING affects real visitors. Confirm such changes with the user first.`

// maxMCPResult keeps tool results within what an agent can use.
const maxMCPResult = 1 << 20

var mcpSchemas = mcp.NewSchemaCache()

type mcpOutput struct {
	Data any `json:"data"`
}

// mcpTools registers the tools a token may use; the others do not exist for it.
type mcpTools struct {
	s   *Server
	srv *mcp.Server
	p   principal
}

func addTool[In any](t mcpTools, scope string, tool *mcp.Tool, h func(context.Context, In) (any, error)) {
	if scope != "" && !access.Has(t.p.Scopes, scope) {
		return
	}
	mcp.AddTool(t.srv, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, mcpOutput, error) {
		data, err := h(ctx, in)
		return nil, mcpOutput{Data: data}, err
	})
}

var closedWorld = new(bool)

func readTool(name, title, description string) *mcp.Tool {
	return &mcp.Tool{Name: name, Title: title, Description: description, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: closedWorld}}
}

// writeTool marks a change; destructive changes remove or overwrite data.
func writeTool(name, title, description string, destructive, idempotent bool) *mcp.Tool {
	return &mcp.Tool{Name: name, Title: title, Description: description, Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: idempotent, OpenWorldHint: closedWorld}}
}

// external marks a tool that talks to a third-party service.
func external(t *mcp.Tool) *mcp.Tool {
	open := true
	t.Annotations.OpenWorldHint = &open
	return t
}

// call runs an HTTP API route in-process as the token, so scopes, project
// limits, validation and audit are the same as for HTTP clients.
func (t mcpTools) call(ctx context.Context, method, path string, q url.Values, body any) (any, error) {
	var rd io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		rd = bytes.NewReader(b)
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	r, e := http.NewRequestWithContext(ctx, method, path, rd)
	if e != nil {
		return nil, e
	}
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "127.0.0.1:0"
	w := httptest.NewRecorder()
	t.s.mux.ServeHTTP(w, r)
	if w.Body.Len() > maxMCPResult {
		return nil, fmt.Errorf("result is %d bytes, too large; narrow the filters or period", w.Body.Len())
	}
	var out any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code >= 400 {
		m, _ := out.(map[string]any)
		msg, _ := m["error"].(string)
		return nil, fmt.Errorf("HTTP %d: %s", w.Code, msg)
	}
	// Unwrap {"data": ...} so the tool result is {"data": ...} like over HTTP.
	if m, ok := out.(map[string]any); ok && len(m) == 1 && m["data"] != nil {
		return m["data"], nil
	}
	return out, nil
}

// project picks the project for a project-level tool.
func (t mcpTools) project(id string) (string, error) {
	if id == "" {
		id = t.p.ProjectID
	}
	if !store.ValidUUID(id) {
		return "", errors.New("valid project_id required; call list_projects")
	}
	return id, nil
}

// path joins escaped segments after /v1/.
func apiPath(parts ...string) string {
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return "/v1/" + strings.Join(parts, "/")
}

type noInput struct{}

func (s *Server) mcpServer(p principal) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "ClickShield", Title: "ClickShield", Version: "2.0.0"}, &mcp.ServerOptions{Instructions: mcpInstructions, SchemaCache: mcpSchemas})
	// The tool list depends on the token, so shared caches must not reuse it.
	srv.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			if list, ok := res.(*mcp.ListToolsResult); ok {
				list.CacheScope = "private"
			}
			return res, err
		}
	})
	t := mcpTools{s: s, srv: srv, p: p}
	addTool(t, "", readTool("whoami", "Token info", "Name, project (empty = all projects) and scopes of the API token in use."), func(context.Context, noInput) (any, error) {
		return map[string]any{"token": p.TokenName, "project_id": p.ProjectID, "scopes": p.Scopes}, nil
	})
	t.analyticsTools()
	t.sessionTools()
	t.projectTools()
	return srv
}

func (s *Server) mcpHandler() http.Handler {
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		p, _ := principalFrom(r.Context())
		return s.mcpServer(p)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host == "" || !strings.EqualFold(u.Host, r.Host) || (u.Scheme != "http" && u.Scheme != "https") {
				writeError(w, 403, "invalid MCP origin")
				return
			}
		}
		p, ok := s.tokenAuth(w, r)
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}
