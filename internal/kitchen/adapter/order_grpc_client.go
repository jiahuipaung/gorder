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

func (g *OrderGRPC) UpdateOrder(ctx context.Context, req *orderpb.Order) error {
	_, err := g.Client.UpdateOrder(ctx, req)
	logrus.Infof("kitchen_adapter.UpdateOrder err:%v", err)
	return err
}
