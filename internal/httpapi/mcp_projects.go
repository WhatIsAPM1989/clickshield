package httpapi

import (
	"context"
	"strconv"

	"github.com/lamooof/clickshield/internal/access"
)

type projectSettingsInput struct {
	Name           string         `json:"name" jsonschema:"Project name"`
	AllowedOrigins []string       `json:"allowed_origins" jsonschema:"Site origins allowed to send events, e.g. https://example.com"`
	Mode           string         `json:"mode,omitempty" jsonschema:"MONITORING (default), NOTIFY, BLOCKING_BOTS or BLOCKING; blocking modes affect real visitors"`
	ScoringConfig  map[string]any `json:"scoring_config,omitempty" jsonschema:"Scoring thresholds and rule weights as returned by list_projects; omit on create for defaults"`
}
type projectUpdateInput struct {
	projectInput
	projectSettingsInput
}
type replaySettingsInput struct {
	projectInput
	Enabled       bool `json:"enabled" jsonschema:"Record sessions; needs S3 configured on the server"`
	SampleRate    int  `json:"sample_rate" jsonschema:"Percent of sessions to record, 0–100"`
	RetentionDays int  `json:"retention_days" jsonschema:"Days to keep recordings, 1–30"`
}
type integrationInput struct {
	projectInput
	Provider         string `json:"provider" jsonschema:"yandex, google or vk"`
	Enabled          bool   `json:"enabled" jsonschema:"Send antifraud_bot / antifraud_human events"`
	CounterID        string `json:"counter_id,omitempty" jsonschema:"yandex: Metrika counter id"`
	MeasurementToken string `json:"measurement_token,omitempty" jsonschema:"yandex: Measurement Protocol token; omit to keep the stored one"`
	MeasurementID    string `json:"measurement_id,omitempty" jsonschema:"google: G-XXXXXXXXXX"`
	APISecret        string `json:"api_secret,omitempty" jsonschema:"google: Measurement Protocol API secret; omit to keep the stored one"`
	PixelID          string `json:"pixel_id,omitempty" jsonschema:"vk: numeric pixel id"`
}
type integrationTestInput struct {
	projectInput
	Provider  string `json:"provider" jsonschema:"yandex, google or vk"`
	SessionID string `json:"session_id" jsonschema:"Session of this project to send as a test"`
}
type metrikaReportInput struct {
	projectInput
	Report string `json:"report" jsonschema:"sources, campaigns, platforms or utm"`
	From   string `json:"from" jsonschema:"Start date YYYY-MM-DD"`
	To     string `json:"to" jsonschema:"End date YYYY-MM-DD, up to 366 days after from"`
	Offset int    `json:"offset,omitempty" jsonschema:"Row offset from 1 for paging"`
}
type metrikaConnectionInput struct {
	projectInput
	CounterID  string `json:"counter_id" jsonschema:"Metrika counter id"`
	OAuthToken string `json:"oauth_token,omitempty" jsonschema:"OAuth token with metrika:read; omit to keep the stored one"`
}

func (t mcpTools) projectCall(ctx context.Context, method, pid string, sub []string, body any) (any, error) {
	pid, err := t.project(pid)
	if err != nil {
		return nil, err
	}
	return t.call(ctx, method, apiPath(append([]string{"projects", pid}, sub...)...), nil, body)
}

func (t mcpTools) projectTools() {
	addTool(t, access.ProjectsRead, readTool("list_projects", "List projects", "Projects with id, mode, allowed origins, public tracker key, replay and scoring settings."), func(ctx context.Context, _ noInput) (any, error) {
		return t.call(ctx, "GET", "/v1/projects", nil, nil)
	})
	addTool(t, access.ProjectsRead, readTool("list_integrations", "List integrations", "Yandex Metrika, GA4 and VK Ads delivery settings of a project and the Metrika read connection. Secrets are never returned, only has_secret."), func(ctx context.Context, in projectInput) (any, error) {
		delivery, err := t.projectCall(ctx, "GET", in.ProjectID, []string{"integrations"}, nil)
		if err != nil {
			return nil, err
		}
		metrika, err := t.projectCall(ctx, "GET", in.ProjectID, []string{"metrika-read"}, nil)
		return map[string]any{"delivery": delivery, "metrika_read": metrika}, err
	})
	addTool(t, access.ProjectsRead, external(readTool("get_metrika_report", "Yandex Metrika report", "Reads a report from the Yandex Metrika counter connected to the project (external request, rate limited).")), func(ctx context.Context, in metrikaReportInput) (any, error) {
		pid, err := t.project(in.ProjectID)
		if err != nil {
			return nil, err
		}
		q := mcpFilter{From: in.From, To: in.To}.query()
		q.Set("report", in.Report)
		if in.Offset > 0 {
			q.Set("offset", strconv.Itoa(in.Offset))
		}
		return t.call(ctx, "GET", apiPath("projects", pid, "metrika-report"), q, nil)
	})
	addTool(t, access.ProjectsRead, readTool("get_operations", "Operational status", "Worker, delivery queue and storage health of the whole installation. Needs an all-projects token."), func(ctx context.Context, _ noInput) (any, error) {
		ops, err := t.call(ctx, "GET", "/v1/operations", nil, nil)
		if err != nil {
			return nil, err
		}
		outbox, err := t.call(ctx, "GET", "/v1/outbox", nil, nil)
		return map[string]any{"operations": ops, "outbox": outbox}, err
	})
	addTool(t, access.ProjectsWrite, writeTool("create_project", "Create a project", "Creates a project in MONITORING mode by default. The result has the public tracker key. Needs an all-projects token.", false, false), func(ctx context.Context, in projectSettingsInput) (any, error) {
		return t.call(ctx, "POST", "/v1/projects", nil, in)
	})
	addTool(t, access.ProjectsWrite, writeTool("update_project", "Update project settings", "Replaces name, origins, mode and scoring_config; read the current values with list_projects and send all of them.", true, true), func(ctx context.Context, in projectUpdateInput) (any, error) {
		return t.projectCall(ctx, "PUT", in.ProjectID, nil, in.projectSettingsInput)
	})
	addTool(t, access.ProjectsWrite, writeTool("update_replay_settings", "Update session recording", "Turns session recording on or off and sets sampling and retention.", false, true), func(ctx context.Context, in replaySettingsInput) (any, error) {
		return t.projectCall(ctx, "PUT", in.ProjectID, []string{"replay"}, map[string]any{"enabled": in.Enabled, "sample_rate": in.SampleRate, "retention_days": in.RetentionDays})
	})
	addTool(t, access.ProjectsWrite, writeTool("update_integration", "Update an integration", "Configures delivery of antifraud events to Yandex Metrika, GA4 or VK Ads. Secrets are stored encrypted and never returned.", false, true), func(ctx context.Context, in integrationInput) (any, error) {
		body := map[string]any{"enabled": in.Enabled, "counter_id": in.CounterID, "measurement_token": in.MeasurementToken, "measurement_id": in.MeasurementID, "api_secret": in.APISecret, "pixel_id": in.PixelID}
		return t.projectCall(ctx, "PUT", in.ProjectID, []string{"integrations", in.Provider}, body)
	})
	addTool(t, access.ProjectsWrite, external(writeTool("test_integration", "Test an integration", "Sends the classification event of one session of this project to the provider now.", false, false)), func(ctx context.Context, in integrationTestInput) (any, error) {
		return t.projectCall(ctx, "POST", in.ProjectID, []string{"integrations", in.Provider, "test"}, map[string]string{"session_id": in.SessionID})
	})
	addTool(t, access.ProjectsWrite, writeTool("set_metrika_connection", "Connect Yandex Metrika reports", "Stores the counter and OAuth token used by get_metrika_report.", false, true), func(ctx context.Context, in metrikaConnectionInput) (any, error) {
		return t.projectCall(ctx, "PUT", in.ProjectID, []string{"metrika-read"}, map[string]string{"counter_id": in.CounterID, "oauth_token": in.OAuthToken})
	})
	addTool(t, access.ProjectsWrite, writeTool("delete_metrika_connection", "Disconnect Yandex Metrika reports", "Deletes the stored Metrika counter and OAuth token.", true, true), func(ctx context.Context, in projectInput) (any, error) {
		return t.projectCall(ctx, "DELETE", in.ProjectID, []string{"metrika-read"}, nil)
	})
}
