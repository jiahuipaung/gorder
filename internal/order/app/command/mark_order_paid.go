package command

import (
	"context"
	"errors"

	"github.com/jiahuipaung/gorder/common/decorator"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/sirupsen/logrus"
)

type MarkOrderPaid struct {
	Order *domain.Order
}

type MarkOrderPaidHandler decorator.CommandHandler[MarkOrderPaid, bool]

type markOrderPaidHandler struct {
	orderRepo domain.Repository
}

func NewMarkOrderPaidHandler(
	orderRepo domain.Repository,
	logger *logrus.Entry,
	metricsClient decorator.MetricsClient,
) MarkOrderPaidHandler {
	if orderRepo == nil {
		panic("orderRepo is nil")
	}
	return decorator.ApplyCommandDecorators[MarkOrderPaid, bool](
		markOrderPaidHandler{orderRepo: orderRepo},
		logger,
		metricsClient,
	)
}

func (h markOrderPaidHandler) Handler(ctx context.Context, cmd MarkOrderPaid) (bool, error) {
	if cmd.Order == nil {
		return false, errors.New("order is required")
	}
	if err := cmd.Order.IsPaid(); err != nil {
		return false, err
	}
	return h.orderRepo.MarkPaid(
		ctx,
		cmd.Order.ID,
		cmd.Order.CustomerID,
		cmd.Order.PaymentLink,
	)
}
