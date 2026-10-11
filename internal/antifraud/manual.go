package antifraud

import (
	"context"
	"fmt"
	"github.com/lamooof/clickshield/internal/domain"
	"time"
)

type ManualWrite struct {
	Context                 domain.SessionContext
	Features                domain.Features
	Score                   domain.Score
	UserID, Note, EventName string
}
type ManualRepository interface {
	SessionContext(context.Context, string) (domain.SessionContext, error)
	SaveManualScore(context.Context, ManualWrite) (int64, error)
}

func Label(ctx context.Context, repository ManualRepository, id, label, note, user string) (int64, error) {
	if label != domain.Human && label != domain.Bot {
		return 0, fmt.Errorf("manual label must be HUMAN or BOT")
	}
	c, err := repository.SessionContext(ctx, id)
	if err != nil {
		return 0, err
	}
	score := domain.Score{Classification: label, Reasons: []domain.Reason{{Code: "MANUAL_OVERRIDE", Details: map[string]any{"note": note}}}}
	if label == domain.Bot {
		score.RiskScore = 100
	}
	return repository.SaveManualScore(ctx, ManualWrite{Context: c, Features: domain.Features{SessionID: id, ComputedAt: time.Now().UTC()}, Score: score, UserID: user, Note: note, EventName: Notification(c.ProjectMode, c.ManualLabel, label, "MANUAL")})
}
