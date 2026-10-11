package analytics

import (
	"context"
	"fmt"
	"time"
)

type Query struct {
	Report                   string
	Filter                   SessionFilter
	Timezone, Event, PageURL string
	Width                    int
}

type ReportReader interface {
	AggregateReport(context.Context, string, string, SessionFilter) (AggregateReport, error)
	BusinessReport(context.Context, SessionFilter) ([]EventMetric, error)
	EventProperties(context.Context, string, SessionFilter) ([]PropertyMetric, error)
	Journey(context.Context, SessionFilter) ([]JourneyEdge, error)
	Retention(context.Context, SessionFilter, string) ([]RetentionCell, error)
	Heatmap(context.Context, SessionFilter, string, int) (HeatmapReport, error)
	Definitions(context.Context, string) ([]AnalyticsDefinition, error)
	FunnelReports(context.Context, []AnalyticsDefinition, SessionFilter) ([]FunnelReport, error)
	Links(context.Context, string, *time.Time, *time.Time) ([]TrackedLink, error)
	Pixels(context.Context, string, *time.Time, *time.Time) ([]TrackingPixel, error)
	Annotations(context.Context, string, *time.Time, *time.Time) ([]Annotation, error)
}

type Service struct {
	Repository *Repository
	reader     ReportReader
	slots      chan struct{}
}

func NewService(repository *Repository) *Service {
	return &Service{Repository: repository, reader: repository, slots: make(chan struct{}, 3)}
}

func (q Query) Validate() error {

	if q.Timezone == "" {
		q.Timezone = "Europe/Moscow"
	}
	if _, err := time.LoadLocation(q.Timezone); err != nil {
		return err
	}
	if q.Filter.From != nil && q.Filter.To != nil && !q.Filter.To.After(*q.Filter.From) {
		return fmt.Errorf("to must be after from")
	}
	switch q.Filter.Class {
	case "", "HUMAN", "SUSPICIOUS", "BOT", "KNOWN_BOT", "UNCLASSIFIED", "NO_BOTS":
	default:
		return fmt.Errorf("invalid classification")
	}
	if q.Width < 0 || q.Width > 20000 {
		return fmt.Errorf("invalid width")
	}
	if !ValidReport(q.Report) {
		switch q.Report {
		case "events", "journey", "retention", "heatmap", "links", "pixels", "annotations":
		case "properties":
			if q.Event == "" {
				return fmt.Errorf("event required")
			}
		case "goals", "funnels":
			if q.Filter.ProjectID == "" {
				return fmt.Errorf("select one project")
			}
		default:
			return fmt.Errorf("unknown report")
		}
	}
	return nil
}

func (s *Service) Report(ctx context.Context, q Query) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := q.Validate(); err != nil {
		return nil, err
	}
	if q.Timezone == "" {
		q.Timezone = "Europe/Moscow"
	}

	if err := s.acquire(ctx); err != nil {
		return nil, err
	}
	defer s.release()
	if ValidReport(q.Report) {
		return s.reader.AggregateReport(ctx, q.Report, q.Timezone, q.Filter)
	}
	switch q.Report {
	case "events":
		return s.reader.BusinessReport(ctx, q.Filter)
	case "properties":
		if q.Event == "" {
			return nil, fmt.Errorf("event required")
		}
		return s.reader.EventProperties(ctx, q.Event, q.Filter)
	case "journey":
		return s.reader.Journey(ctx, q.Filter)
	case "retention":
		return s.reader.Retention(ctx, q.Filter, q.Timezone)
	case "heatmap":
		return s.reader.Heatmap(ctx, q.Filter, q.PageURL, q.Width)
	case "goals", "funnels":
		if q.Filter.ProjectID == "" {
			return nil, fmt.Errorf("select one project")
		}
		defs, err := s.reader.Definitions(ctx, q.Filter.ProjectID)
		if err != nil {
			return nil, err
		}
		selected := []AnalyticsDefinition{}
		for _, d := range defs {
			if (q.Report == "goals" && d.Kind == "goal") || (q.Report == "funnels" && d.Kind == "funnel") {
				selected = append(selected, d)
			}
		}
		return s.reader.FunnelReports(ctx, selected, q.Filter)
	case "links":
		return s.reader.Links(ctx, q.Filter.ProjectID, q.Filter.From, q.Filter.To)
	case "pixels":
		return s.reader.Pixels(ctx, q.Filter.ProjectID, q.Filter.From, q.Filter.To)
	case "annotations":
		return s.reader.Annotations(ctx, q.Filter.ProjectID, q.Filter.From, q.Filter.To)
	default:
		return nil, fmt.Errorf("unknown report")
	}
}
