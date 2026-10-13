package httpapi

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/lamooof/clickshield/internal/access"
	"github.com/lamooof/clickshield/internal/analytics"
)

// mcpFilter mirrors the session filter of the HTTP API.
type mcpFilter struct {
	ProjectID      string            `json:"project_id,omitempty" jsonschema:"Project UUID. Optional for a project token; omit with an all-projects token to cover every project"`
	From           string            `json:"from,omitempty" jsonschema:"Inclusive start, RFC3339 with timezone, e.g. 2026-10-01T00:00:00+03:00"`
	To             string            `json:"to,omitempty" jsonschema:"Exclusive end, RFC3339 with timezone"`
	Classification string            `json:"classification,omitempty" jsonschema:"HUMAN, SUSPICIOUS, BOT, KNOWN_BOT, UNCLASSIFIED or NO_BOTS"`
	Source         string            `json:"source,omitempty" jsonschema:"Traffic source, e.g. yandex"`
	Channel        string            `json:"channel,omitempty" jsonschema:"Traffic channel, e.g. paid, organic, direct"`
	Campaign       string            `json:"campaign,omitempty" jsonschema:"utm_campaign value"`
	Device         string            `json:"device,omitempty" jsonschema:"desktop, mobile, tablet or unknown"`
	LandingURL     string            `json:"landing_url,omitempty" jsonschema:"Landing page URL without query"`
	Bot            string            `json:"bot,omitempty" jsonschema:"Known bot name"`
	BotCategory    string            `json:"bot_category,omitempty" jsonschema:"Bot category: search, ai, social, seo, ads, monitoring, feeds, security, automation, other"`
	PaidOnly       bool              `json:"paid_only,omitempty" jsonschema:"Only sessions with an ad click id"`
	Dimensions     map[string]string `json:"dimensions,omitempty" jsonschema:"More filters by dimension: referrer, utm_source, utm_medium, utm_campaign, utm_content, utm_term, exit_page, os, browser, country, region, city, language, network"`
}

func (f mcpFilter) query() url.Values {
	q := url.Values{}
	for k, v := range map[string]string{"project_id": f.ProjectID, "from": f.From, "to": f.To, "classification": f.Classification, "source": f.Source, "channel": f.Channel, "campaign": f.Campaign, "device": f.Device, "landing_url": f.LandingURL, "bot": f.Bot, "bot_category": f.BotCategory} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if f.PaidOnly {
		q.Set("paid_only", "true")
	}
	for k, v := range f.Dimensions {
		q.Set(k, v)
	}
	return q
}

type reportInput struct {
	mcpFilter
	Report   string `json:"report" jsonschema:"traffic; aggregate: sources, referrers, campaigns, utm, terms, landing, exit, devices, browsers, geo, languages, networks, quality, bots, daily, hours, activity; product: events, properties, goals, funnels, journey, retention, heatmap, links, pixels, annotations"`
	Timezone string `json:"timezone,omitempty" jsonschema:"IANA zone for daily/hourly buckets and cohorts, default Europe/Moscow"`
	Event    string `json:"event,omitempty" jsonschema:"Event name, required for properties"`
	PageURL  string `json:"page_url,omitempty" jsonschema:"Page URL, required for heatmap"`
	Width    int    `json:"width,omitempty" jsonschema:"Viewport width for heatmap"`
}
type timelineInput struct {
	mcpFilter
	Report    string   `json:"report,omitempty" jsonschema:"Aggregate report whose rows become series, default sources"`
	Interval  string   `json:"interval" jsonschema:"hour, day, week or month"`
	Series    []string `json:"series,omitempty" jsonschema:"Up to 20 row keys from the same report to plot"`
	TotalOnly bool     `json:"total_only,omitempty" jsonschema:"Only the total line"`
	Timezone  string   `json:"timezone,omitempty" jsonschema:"IANA zone, default Europe/Moscow"`
}
type projectInput struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"Project UUID; optional for a project token"`
}
type definitionInput struct {
	projectInput
	ID            string                    `json:"id,omitempty" jsonschema:"Definition id to update; omit to create"`
	Kind          string                    `json:"kind" jsonschema:"goal (exactly one step) or funnel (2–12 ordered steps)"`
	Name          string                    `json:"name" jsonschema:"Name, up to 120 characters"`
	Steps         []analytics.AnalyticsStep `json:"steps" jsonschema:"Steps: type page, event or outcome; value is the URL, event name or outcome status; match exact or prefix; optional event properties to match"`
	WindowMinutes int                       `json:"window_minutes" jsonschema:"Conversion window, 1–10080 minutes"`
}
type itemInput struct {
	projectInput
	ID string `json:"id" jsonschema:"Item id"`
}
type campaignListInput struct {
	projectInput
	Kind string `json:"kind" jsonschema:"links, pixels or annotations"`
	From string `json:"from,omitempty" jsonschema:"RFC3339 period start for hit counts and annotations"`
	To   string `json:"to,omitempty" jsonschema:"RFC3339 period end"`
}
type campaignInput struct {
	projectInput
	Kind        string `json:"kind" jsonschema:"links, pixels or annotations"`
	ID          string `json:"id,omitempty" jsonschema:"Item id to update; omit to create"`
	Name        string `json:"name,omitempty" jsonschema:"Link or pixel name"`
	Destination string `json:"destination,omitempty" jsonschema:"Link target, http(s) URL; UTM parameters are kept"`
	Slug        string `json:"slug,omitempty" jsonschema:"Short link slug (3–80 of a-z, 0-9, _ and -); served at /l/{slug}"`
	Enabled     *bool  `json:"enabled,omitempty" jsonschema:"Link or pixel works; default true"`
	At          string `json:"at,omitempty" jsonschema:"Annotation time, RFC3339"`
	Text        string `json:"text,omitempty" jsonschema:"Annotation text"`
}
type campaignDeleteInput struct {
	itemInput
	Kind string `json:"kind" jsonschema:"links, pixels or annotations"`
}

func (t mcpTools) analyticsTools() {
	addTool(t, access.AnalyticsRead, readTool("analytics_report", "Analytics report", "Traffic, aggregate and product analytics of sessions. Dates select sessions or first-visit cohorts. journey returns the top 200 edges, properties the top 500 values; retention includes returns after the cohort period."), func(ctx context.Context, in reportInput) (any, error) {
		q := in.query()
		for k, v := range map[string]string{"timezone": in.Timezone, "event": in.Event, "page_url": in.PageURL} {
			if v != "" {
				q.Set(k, v)
			}
		}
		if in.Width > 0 {
			q.Set("width", strconv.Itoa(in.Width))
		}
		if in.Report == "traffic" {
			return t.call(ctx, "GET", "/v1/traffic", q, nil)
		}
		return t.call(ctx, "GET", apiPath("analytics", in.Report), q, nil)
	})
	addTool(t, access.AnalyticsRead, readTool("report_timeline", "Report timeline", "Time series of an aggregate report. from and to are required; long periods need a coarser interval."), func(ctx context.Context, in timelineInput) (any, error) {
		q := in.query()
		q.Set("interval", in.Interval)
		q["series"] = in.Series
		for k, v := range map[string]string{"report": in.Report, "timezone": in.Timezone} {
			if v != "" {
				q.Set(k, v)
			}
		}
		if in.TotalOnly {
			q.Set("total_only", "true")
		}
		return t.call(ctx, "GET", "/v1/reports/timeline", q, nil)
	})
	addTool(t, access.AnalyticsRead, readTool("list_definitions", "List goals and funnels", "Goals and funnels configured for a project."), func(ctx context.Context, in projectInput) (any, error) {
		pid, err := t.project(in.ProjectID)
		if err != nil {
			return nil, err
		}
		return t.call(ctx, "GET", apiPath("projects", pid, "definitions"), nil, nil)
	})
	addTool(t, access.AnalyticsRead, readTool("list_campaign_tools", "List links, pixels or annotations", "Short links and tracking pixels with hit counts, or chart annotations, for a project."), func(ctx context.Context, in campaignListInput) (any, error) {
		pid, err := t.project(in.ProjectID)
		if err != nil {
			return nil, err
		}
		return t.call(ctx, "GET", apiPath("projects", pid, "tools", in.Kind), mcpFilter{From: in.From, To: in.To}.query(), nil)
	})
	addTool(t, access.AnalyticsWrite, writeTool("save_definition", "Create or update a goal or funnel", "Creates a goal or funnel, or replaces an existing one when id is given. Reports recompute from stored events.", false, true), func(ctx context.Context, in definitionInput) (any, error) {
		pid, err := t.project(in.ProjectID)
		if err != nil {
			return nil, err
		}
		body := analytics.AnalyticsDefinition{Kind: in.Kind, Name: in.Name, Steps: in.Steps, WindowMinutes: in.WindowMinutes}
		if in.ID != "" {
			return t.call(ctx, "PUT", apiPath("projects", pid, "definitions", in.ID), nil, body)
		}
		return t.call(ctx, "POST", apiPath("projects", pid, "definitions"), nil, body)
	})
	addTool(t, access.AnalyticsWrite, writeTool("delete_definition", "Delete a goal or funnel", "Deletes a goal or funnel definition. Stored events are kept.", true, true), func(ctx context.Context, in itemInput) (any, error) {
		pid, err := t.project(in.ProjectID)
		if err != nil {
			return nil, err
		}
		return t.call(ctx, "DELETE", apiPath("projects", pid, "definitions", in.ID), nil, nil)
	})
	addTool(t, access.AnalyticsWrite, writeTool("save_campaign_tool", "Create or update a link, pixel or annotation", "links: name, destination, optional slug and enabled. pixels: name, enabled; the result has the pixel id for /p/{id}. annotations: at and text. With id the item is updated.", false, true), func(ctx context.Context, in campaignInput) (any, error) {
		pid, err := t.project(in.ProjectID)
		if err != nil {
			return nil, err
		}
		enabled := in.Enabled == nil || *in.Enabled
		var body any
		switch in.Kind {
		case "links":
			body = map[string]any{"name": in.Name, "destination": in.Destination, "slug": in.Slug, "enabled": enabled}
		case "pixels":
			body = map[string]any{"name": in.Name, "enabled": enabled}
		case "annotations":
			body = map[string]any{"at": in.At, "text": in.Text}
		default:
			return nil, errors.New("kind must be links, pixels or annotations")
		}
		if in.ID != "" {
			return t.call(ctx, "PUT", apiPath("projects", pid, "tools", in.Kind, in.ID), nil, body)
		}
		return t.call(ctx, "POST", apiPath("projects", pid, "tools", in.Kind), nil, body)
	})
	addTool(t, access.AnalyticsWrite, writeTool("delete_campaign_tool", "Delete a link, pixel or annotation", "Deletes the item; a deleted link or pixel stops working.", true, true), func(ctx context.Context, in campaignDeleteInput) (any, error) {
		pid, err := t.project(in.ProjectID)
		if err != nil {
			return nil, err
		}
		return t.call(ctx, "DELETE", apiPath("projects", pid, "tools", in.Kind, in.ID), nil, nil)
	})
}
