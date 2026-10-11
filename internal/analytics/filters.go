package analytics

import (
	"fmt"
	"strings"
	"time"
)

// SessionFilter narrows session listings. Empty fields do not filter.
type SessionFilter struct {
	DeviceSet   bool
	Device      string
	CampaignSet bool
	Source      string
	Channel     string
	Campaign    string
	LandingURL  string
	ReplayOnly  bool
	From        *time.Time
	To          *time.Time
	ProjectID   string
	Class       string
	PaidOnly    bool
	BotName     string
	BotCategory string
	// Dims narrows by exact values of filterable report dimensions.
	Dims map[string]string
}

func sessionWhere(filter SessionFilter) (string, []any) {
	args := []any{}
	where := []string{"1=1"}
	if filter.ProjectID != "" {
		args = append(args, filter.ProjectID)
		where = append(where, fmt.Sprintf("s.project_id=$%d", len(args)))
	}
	if filter.Class == "UNCLASSIFIED" {
		where = append(where, "s.current_classification=''")
	} else if filter.Class == "NO_BOTS" {
		where = append(where, "s.current_classification NOT IN ('BOT','KNOWN_BOT')")
	} else if filter.Class != "" {
		args = append(args, filter.Class)
		where = append(where, fmt.Sprintf("s.current_classification=$%d", len(args)))
	}
	if filter.BotName != "" {
		args = append(args, filter.BotName)
		where = append(where, fmt.Sprintf("s.bot_name=$%d", len(args)))
	}
	if filter.BotCategory != "" {
		args = append(args, filter.BotCategory)
		where = append(where, fmt.Sprintf("s.bot_category=$%d AND s.bot_name<>''", len(args)))
	}
	if filter.PaidOnly {
		where = append(where, `s.traffic_channel IN ('paid','paid_social','display')`)
	}

	for _, pair := range []struct{ column, value string }{{"traffic_source", filter.Source}, {"traffic_channel", filter.Channel}} {
		if pair.value != "" {
			args = append(args, pair.value)
			where = append(where, fmt.Sprintf("s.%s=$%d", pair.column, len(args)))
		}
	}
	if filter.LandingURL != "" {
		args = append(args, filter.LandingURL)
		where = append(where, fmt.Sprintf("split_part(s.landing_url,'?',1)=$%d", len(args)))
	}
	if filter.CampaignSet {
		args = append(args, filter.Campaign)
		where = append(where, fmt.Sprintf("s.utm_campaign=$%d", len(args)))
	}
	if filter.DeviceSet {
		args = append(args, filter.Device)
		where = append(where, fmt.Sprintf("COALESCE(NULLIF(s.device_type,''),'unknown')=$%d", len(args)))
	}
	if filter.From != nil {
		args = append(args, *filter.From)
		where = append(where, fmt.Sprintf("s.started_at >= $%d", len(args)))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		where = append(where, fmt.Sprintf("s.started_at < $%d", len(args)))
	}
	if filter.ReplayOnly {
		where = append(where, "EXISTS(SELECT 1 FROM analytics.replay_recordings rr WHERE rr.session_id=s.id AND rr.expires_at>now())")
	}
	var dims []string
	dims, args = dimensionWhere(filter.Dims, args)
	where = append(where, dims...)
	return strings.Join(where, " AND "), args
}

func Where(f SessionFilter) (string, []any) { return sessionWhere(f) }
