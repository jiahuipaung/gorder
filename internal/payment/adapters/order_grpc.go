package adapters

import (
	"context"

	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/sirupsen/logrus"
)

type OrderGrpc struct {
	client orderpb.OrderServiceClient
}

func NewOrderGrpc(client orderpb.OrderServiceClient) *OrderGrpc {
	return &OrderGrpc{client: client}
}

func (o *OrderGrpc) UpdateOrder(ctx context.Context, order *orderpb.Order) error {
	_, err := o.client.UpdateOrder(ctx, order)
	logrus.Infof("payment_adapters.UpdateOrder.UpdateOrder err:%v", err)
	return err
}
