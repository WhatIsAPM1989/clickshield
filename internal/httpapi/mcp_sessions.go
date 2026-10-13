package httpapi

import (
	"context"
	"strconv"

	"github.com/lamooof/clickshield/internal/access"
)

type sessionListInput struct {
	mcpFilter
	ReplayOnly bool `json:"replay_only,omitempty" jsonschema:"Only sessions with a recording"`
	Limit      int  `json:"limit,omitempty" jsonschema:"Page size 1–200, default 100"`
	Offset     int  `json:"offset,omitempty" jsonschema:"Rows to skip"`
}
type sessionInput struct {
	SessionID string `json:"session_id" jsonschema:"Session UUID"`
}
type labelInput struct {
	sessionInput
	Label string `json:"label" jsonschema:"BOT or HUMAN"`
	Note  string `json:"note,omitempty" jsonschema:"Reason, up to 500 characters"`
}
type outcomeInput struct {
	sessionInput
	ExternalID string `json:"external_id" jsonschema:"Lead id in the CRM, up to 128 characters"`
	Status     string `json:"status" jsonschema:"QUALIFIED, SPAM or LOST"`
}

func (t mcpTools) session(ctx context.Context, method, sid, sub string, body any) (any, error) {
	parts := []string{"sessions", sid}
	if sub != "" {
		parts = append(parts, sub)
	}
	return t.call(ctx, method, apiPath(parts...), nil, body)
}

func (t mcpTools) sessionTools() {
	addTool(t, access.SessionsRead, readTool("list_sessions", "List sessions", "Visits with classification, risk score, source, device, ASN and ad click ids, newest first. Contains personal data (IP, fingerprint)."), func(ctx context.Context, in sessionListInput) (any, error) {
		q := in.query()
		if in.ReplayOnly {
			q.Set("replay_only", "true")
		}
		if in.Limit > 0 {
			q.Set("limit", strconv.Itoa(in.Limit))
		}
		q.Set("offset", strconv.Itoa(in.Offset))
		return t.call(ctx, "GET", "/v1/sessions", q, nil)
	})
	addTool(t, access.SessionsRead, readTool("get_session", "Session details", "One session with risk reasons, features, pages, events, classifications and lead outcomes."), func(ctx context.Context, in sessionInput) (any, error) {
		return t.session(ctx, "GET", in.SessionID, "", nil)
	})
	addTool(t, access.SessionsRead, readTool("get_stats", "Session statistics", "Counts by classification and average risk for the filter."), func(ctx context.Context, in mcpFilter) (any, error) {
		return t.call(ctx, "GET", "/v1/stats", in.query(), nil)
	})
	addTool(t, access.SessionsRead, readTool("get_bot_stats", "Known bot statistics", "Sessions per known crawler and bot category."), func(ctx context.Context, in projectInput) (any, error) {
		return t.call(ctx, "GET", "/v1/bots", mcpFilter{ProjectID: in.ProjectID}.query(), nil)
	})
	addTool(t, access.SessionsRead, readTool("get_lead_quality", "Lead quality", "Lead outcomes (QUALIFIED, SPAM, LOST) against session classification, to check scoring precision."), func(ctx context.Context, in projectInput) (any, error) {
		return t.call(ctx, "GET", "/v1/quality", mcpFilter{ProjectID: in.ProjectID}.query(), nil)
	})
	addTool(t, access.SessionsWrite, writeTool("label_session", "Label a session", "Manual BOT or HUMAN label. It overrides the score and is sent to connected ad and analytics integrations.", false, true), func(ctx context.Context, in labelInput) (any, error) {
		return t.session(ctx, "POST", in.SessionID, "label", map[string]string{"label": in.Label, "note": in.Note})
	})
	addTool(t, access.SessionsWrite, writeTool("set_lead_outcome", "Set a lead outcome", "Records the CRM result of a lead from this session; used for lead quality and conversions.", false, true), func(ctx context.Context, in outcomeInput) (any, error) {
		return t.session(ctx, "POST", in.SessionID, "outcomes", map[string]string{"session_id": in.SessionID, "external_id": in.ExternalID, "status": in.Status})
	})
	addTool(t, access.ReplayRead, readTool("get_session_replay", "Session recording", "rrweb recording of a session by page. Large recordings are refused; open them in the panel instead."), func(ctx context.Context, in sessionInput) (any, error) {
		return t.session(ctx, "GET", in.SessionID, "replay", nil)
	})
	addTool(t, access.ReplayWrite, writeTool("delete_session_replay", "Delete a session recording", "Deletes the recording of a session permanently.", true, true), func(ctx context.Context, in sessionInput) (any, error) {
		return t.session(ctx, "DELETE", in.SessionID, "replay", nil)
	})
}
