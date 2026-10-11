package service

import (
	"context"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/integration"
)

type Sender interface {
	Send(context.Context, domain.Delivery) integration.Outcome
	Test(context.Context, domain.Delivery) integration.Outcome
}
