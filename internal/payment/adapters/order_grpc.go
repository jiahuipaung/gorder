package adapters

import (
	"context"
	"github.com/jiahuipaung/gorder/common/tracing"

	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/sirupsen/logrus"
)

type OrderGrpc struct {
	client orderpb.OrderServiceClient
}

func NewOrderGrpc(client orderpb.OrderServiceClient) *OrderGrpc {
	return &OrderGrpc{client: client}
}

func (o *OrderGrpc) AttachPaymentSession(
	ctx context.Context,
	orderID, customerID, paymentLink string,
) error {
	ctx, span := tracing.Start(ctx, "OrderGrpc::AttachPaymentSession")
	defer span.End()

	_, err := o.client.AttachPaymentSession(ctx, &orderpb.AttachPaymentSessionRequest{
		OrderID:     orderID,
		CustomerID:  customerID,
		PaymentLink: paymentLink,
	})
	logrus.Infof("payment_adapters.AttachPaymentSession err:%v", err)
	return err
}
