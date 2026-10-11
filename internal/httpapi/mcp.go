package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lamooof/clickshield/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpQuery struct {
	Report         string `json:"report" jsonschema:"Report name: a traffic preset or events, properties, goals, funnels, journey, retention, heatmap, links, pixels, annotations"`
	From           string `json:"from,omitempty" jsonschema:"Inclusive RFC3339 cohort/session start"`
	To             string `json:"to,omitempty" jsonschema:"Exclusive RFC3339 cohort/session end"`
	Classification string `json:"classification,omitempty" jsonschema:"HUMAN, SUSPICIOUS, BOT, KNOWN_BOT, UNCLASSIFIED or NO_BOTS"`
	Channel        string `json:"channel,omitempty"`
	Source         string `json:"source,omitempty"`
	Timezone       string `json:"timezone,omitempty"`
	Event          string `json:"event,omitempty"`
	PageURL        string `json:"page_url,omitempty"`
	Width          int    `json:"width,omitempty"`
}
type mcpOutput struct {
	Data any `json:"data"`
}

func (s *Server) mcpReport(ctx context.Context, req *mcp.CallToolRequest, q mcpQuery) (*mcp.CallToolResult, mcpOutput, error) {
	pid, ok := ctx.Value(analyticsProjectKey{}).(string)
	if !ok || !store.ValidUUID(pid) {
		return nil, mcpOutput{}, errors.New("authenticated project required")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	f := store.SessionFilter{ProjectID: pid, Class: q.Classification, Source: q.Source, Channel: q.Channel}
	switch f.Class {
	case "", "HUMAN", "SUSPICIOUS", "BOT", "KNOWN_BOT", "UNCLASSIFIED", "NO_BOTS":
	default:
		return nil, mcpOutput{}, errors.New("invalid classification")
	}
	for _, v := range []struct {
		raw string
		dst **time.Time
	}{{q.From, &f.From}, {q.To, &f.To}} {
		if v.raw != "" {
			t, e := time.Parse(time.RFC3339, v.raw)
			if e != nil {
				return nil, mcpOutput{}, e
			}
			*v.dst = &t
		}
	}
	if f.From != nil && f.To != nil && !f.To.After(*f.From) {
		return nil, mcpOutput{}, errors.New("to must be after from")
	}
	zone := q.Timezone
	if zone == "" {
		zone = "Europe/Moscow"
	}
	if _, e := time.LoadLocation(zone); e != nil {
		return nil, mcpOutput{}, e
	}
	var data any
	var err error
	if store.ValidReport(q.Report) {
		data, err = s.store.AggregateReport(ctx, q.Report, zone, f)
	} else {
		switch q.Report {
		case "events":
			data, err = s.store.BusinessReport(ctx, f)
		case "properties":
			if q.Event == "" {
				return nil, mcpOutput{}, errors.New("event required")
			}
			data, err = s.store.EventProperties(ctx, q.Event, f)
		case "journey":
			data, err = s.store.Journey(ctx, f)
		case "retention":
			data, err = s.store.Retention(ctx, f, zone)
		case "heatmap":
			if q.Width < 0 || q.Width > 20000 {
				return nil, mcpOutput{}, errors.New("invalid width")
			}
			data, err = s.store.Heatmap(ctx, f, q.PageURL, q.Width)
		case "goals", "funnels":
			defs, e := s.store.Definitions(ctx, pid)
			err = e
			reports := []store.FunnelReport{}
			if err == nil {
				for _, d := range defs {
					if (q.Report == "goals" && d.Kind == "goal") || (q.Report == "funnels" && d.Kind == "funnel") {
						r, e := s.store.FunnelReport(ctx, d, f)
						if e != nil {
							err = e
							break
						}
						reports = append(reports, r)
					}
				}
			}
			data = reports
		case "links":
			data, err = s.store.Links(ctx, pid, f.From, f.To)
		case "pixels":
			data, err = s.store.Pixels(ctx, pid, f.From, f.To)
		case "annotations":
			data, err = s.store.Annotations(ctx, pid, f.From, f.To)
		default:
			return nil, mcpOutput{}, errors.New("unknown report")
		}
	}
	if err != nil {
		s.log.Error("MCP report", "error", err)
		return nil, mcpOutput{}, errors.New("could not load report")
	}
	return nil, mcpOutput{Data: data}, nil
}
func (s *Server) mcpHandler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "ClickShield Analytics", Version: "1.0.0"}, nil)
	closed := false
	mcp.AddTool(server, &mcp.Tool{Name: "analytics_report", Description: "Read project analytics, traffic, events, goals, funnels, journeys, heatmaps, retention and campaign tools. API key fixes the project. Dates select sessions or first-visit cohorts. Journey is top 200 edges, properties top 500 values. Retention uses retained history and includes returns beyond cohort date range.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &closed}}, s.mcpReport)
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host == "" || !strings.EqualFold(u.Host, r.Host) || (u.Scheme != "http" && u.Scheme != "https") {
				writeError(w, 403, "invalid MCP origin")
				return
			}
		}
		pid, ok := s.bearerProject(w, r)
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), analyticsProjectKey{}, pid)))
	})
}
