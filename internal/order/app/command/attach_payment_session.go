package command

import (
	"context"
	"errors"

	"github.com/jiahuipaung/gorder/common/decorator"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/sirupsen/logrus"
)

type AttachPaymentSession struct {
	OrderID     string
	CustomerID  string
	PaymentLink string
}

type AttachPaymentSessionHandler decorator.CommandHandler[AttachPaymentSession, bool]

type attachPaymentSessionHandler struct {
	orderRepo domain.Repository
}

func NewAttachPaymentSessionHandler(
	orderRepo domain.Repository,
	logger *logrus.Entry,
	metricsClient decorator.MetricsClient,
) AttachPaymentSessionHandler {
	if orderRepo == nil {
		panic("orderRepo is nil")
	}
	return decorator.ApplyCommandDecorators[AttachPaymentSession, bool](
		attachPaymentSessionHandler{orderRepo: orderRepo},
		logger,
		metricsClient,
	)
}

func (h attachPaymentSessionHandler) Handler(
	ctx context.Context,
	cmd AttachPaymentSession,
) (bool, error) {
	if cmd.OrderID == "" {
		return false, errors.New("order ID is required")
	}
	if cmd.CustomerID == "" {
		return false, errors.New("customer ID is required")
	}
	if cmd.PaymentLink == "" {
		return false, errors.New("payment link is required")
	}
	return h.orderRepo.AttachPaymentSession(
		ctx,
		cmd.OrderID,
		cmd.CustomerID,
		cmd.PaymentLink,
	)
}
