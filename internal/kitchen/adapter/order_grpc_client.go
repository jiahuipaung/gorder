package adapter

import (
	"context"
	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/sirupsen/logrus"
)

type OrderGRPC struct {
	Client orderpb.OrderServiceClient
}

func NewOrderGRPC(client orderpb.OrderServiceClient) *OrderGRPC {
	return &OrderGRPC{Client: client}
}

func (g *OrderGRPC) MarkOrderReady(ctx context.Context, orderID, customerID string) error {
	_, err := g.Client.MarkOrderReady(ctx, &orderpb.MarkOrderReadyRequest{
		OrderID:    orderID,
		CustomerID: customerID,
	})
	logrus.Infof("kitchen_adapter.MarkOrderReady err:%v", err)
	return err
}

func (g *OrderGRPC) GetStatus(ctx context.Context, orderID, customerID string) (string, error) {
	o, err := g.Client.GetOrder(ctx, &orderpb.GetOrderRequest{OrderID: orderID, CustomerID: customerID})
	if err != nil {
		return "", err
	}
	return o.Status, nil
}
