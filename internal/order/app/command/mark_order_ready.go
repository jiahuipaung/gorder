package command

import (
	"context"
	"errors"

	"github.com/jiahuipaung/gorder/common/decorator"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/sirupsen/logrus"
)

type MarkOrderReady struct {
	OrderID    string
	CustomerID string
}

type MarkOrderReadyHandler decorator.CommandHandler[MarkOrderReady, bool]

type markOrderReadyHandler struct {
	orderRepo domain.Repository
}

func NewMarkOrderReadyHandler(
	orderRepo domain.Repository,
	logger *logrus.Entry,
	metricsClient decorator.MetricsClient,
) MarkOrderReadyHandler {
	if orderRepo == nil {
		panic("orderRepo is nil")
	}
	return decorator.ApplyCommandDecorators[MarkOrderReady, bool](
		markOrderReadyHandler{orderRepo: orderRepo},
		logger,
		metricsClient,
	)
}

func (h markOrderReadyHandler) Handler(ctx context.Context, cmd MarkOrderReady) (bool, error) {
	if cmd.OrderID == "" {
		return false, errors.New("order ID is required")
	}
	if cmd.CustomerID == "" {
		return false, errors.New("customer ID is required")
	}
	return h.orderRepo.MarkReady(ctx, cmd.OrderID, cmd.CustomerID)
}
