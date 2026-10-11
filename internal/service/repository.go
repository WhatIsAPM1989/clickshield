package service

import (
	"context"
	"github.com/lamooof/clickshield/internal/antifraud"
	"github.com/lamooof/clickshield/internal/domain"
	"time"
)

// Repository is the worker/application boundary. Individual antifraud use cases
// consume smaller interfaces, so policy tests do not require PostgreSQL.
type Repository interface {
	ProjectAnalytics(context.Context) (bool, error)
	antifraud.ScoreRepository
	antifraud.BatchRepository
	antifraud.ManualRepository
	SessionContext(context.Context, string) (domain.SessionContext, error)
	ClaimDueScore(context.Context, time.Duration, time.Duration) (antifraud.ScoreClaim, bool, error)
	ClaimOutbox(context.Context) (domain.OutboxClaim, bool, error)
	Delivery(context.Context, int64) (domain.Delivery, error)
	FinishDelivery(context.Context, domain.OutboxClaim, string, string, time.Duration) error
	ResetStuckOutbox(context.Context) error
	Cleanup(context.Context, time.Duration, time.Duration) error
	Operations(context.Context) (domain.Operations, error)
}
