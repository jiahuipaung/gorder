package grpc

import (
	"context"

	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
	"github.com/sirupsen/logrus"
)

type StockGrpc struct {
	client stockpb.StockServiceClient
}

func NewStockGrpc(client stockpb.StockServiceClient) *StockGrpc {
	return &StockGrpc{client: client}
}

func (s StockGrpc) CheckIfItemsInStock(ctx context.Context, items []*orderpb.ItemWithQuantity) (*stockpb.CheckIfItemsInStockResponse, error) {
	resp, err := s.client.CheckIfItemsInStock(ctx, &stockpb.CheckIfItemsInStockRequest{
		Items: items,
	})
	if err != nil {
		return nil, err
	}
	logrus.Infof("StockGrpc CheckIfItemsInStock resp: %v", resp)
	return resp, nil
}

func (s StockGrpc) GetItem(ctx context.Context, itemIDs []string) ([]*orderpb.Item, error) {
	resp, err := s.client.GetItems(ctx, &stockpb.GetItemsRequest{
		ItemIDs: itemIDs,
	})
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}
