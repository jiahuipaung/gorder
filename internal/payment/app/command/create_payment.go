package command

import (
	"context"
	"github.com/jiahuipaung/gorder/common/decorator"
	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/common/tracing"
	"github.com/jiahuipaung/gorder/payment/domain"
	"github.com/sirupsen/logrus"
)

type CreatePayment struct {
	Order *orderpb.Order
}

type CreatePaymentHandler decorator.CommandHandler[CreatePayment, string]

type createPaymentHandler struct {
	processor domain.Processor
	orderGrpc OrderService
}

func (c createPaymentHandler) Handler(ctx context.Context, cmd CreatePayment) (string, error) {
	ctx, span := tracing.Start(ctx, "create_payment")
	defer span.End()
	if cmd.Order == nil {
		logrus.Errorf("Order field is nil")
	}
	link, err := c.processor.CreatePaymentLink(ctx, cmd.Order)
	if err != nil {
		return "", err
	}
	logrus.Infof("create payment link for order: %s success, link:%s", cmd.Order.ID, link)
	newOrder := &orderpb.Order{
		ID:          cmd.Order.ID,
		CustomerID:  cmd.Order.CustomerID,
		Status:      "waiting_for_payment",
		PaymentLink: link,
		Items:       cmd.Order.Items,
	}
	err = c.orderGrpc.UpdateOrder(ctx, newOrder)
	return link, nil
}

func NewCreatePaymentHandler(
	processor domain.Processor,
	orderGrpc OrderService,
	logger *logrus.Entry,
	metricsClient decorator.MetricsClient,
) CreatePaymentHandler {
	return decorator.ApplyCommandDecorators[CreatePayment, string](
		createPaymentHandler{
			processor: processor,
			orderGrpc: orderGrpc,
		},
		logger,
		metricsClient,
	)
}
