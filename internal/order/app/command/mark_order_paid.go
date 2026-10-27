package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/jiahuipaung/gorder/common/decorator"
	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/sirupsen/logrus"
)

type MarkOrderPaid struct {
	Order *domain.Order
}

type MarkOrderPaidHandler decorator.CommandHandler[MarkOrderPaid, bool]

type markOrderPaidHandler struct {
	orderRepo domain.Repository
	stock     ReservationVerifier
}

type ReservationVerifier interface {
	GetReservation(context.Context, string, string) (*stockpb.ReservationResponse, error)
}

func NewMarkOrderPaidHandler(
	orderRepo domain.Repository,
	stock ReservationVerifier,
	logger *logrus.Entry,
	metricsClient decorator.MetricsClient,
) MarkOrderPaidHandler {
	if orderRepo == nil {
		panic("orderRepo is nil")
	}
	if stock == nil {
		panic("reservation verifier is nil")
	}
	return decorator.ApplyCommandDecorators[MarkOrderPaid, bool](
		markOrderPaidHandler{orderRepo: orderRepo, stock: stock},
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
	r, err := h.stock.GetReservation(ctx, cmd.Order.ID, cmd.Order.CustomerID)
	if err != nil {
		return false, err
	}
	if r == nil || r.ReservationId != cmd.Order.ID || r.CustomerId != cmd.Order.CustomerID || r.Status != "CONFIRMED" {
		return false, fmt.Errorf("paid order requires confirmed inventory reservation")
	}
	return h.orderRepo.MarkPaid(
		ctx,
		cmd.Order.ID,
		cmd.Order.CustomerID,
		cmd.Order.PaymentLink,
	)
}
