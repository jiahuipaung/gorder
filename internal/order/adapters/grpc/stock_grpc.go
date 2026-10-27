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

func (s StockGrpc) Reserve(ctx context.Context, orderID, customerID string, items []*orderpb.ItemWithQuantity, expiresAtUnixMillis int64) (*stockpb.ReservationResponse, error) {
	return s.client.Reserve(ctx, &stockpb.ReserveRequest{ReservationId: orderID, CustomerId: customerID, Items: items, ExpiresAtUnixMillis: expiresAtUnixMillis})
}

func (s StockGrpc) GetReservation(ctx context.Context, id, customer string) (*stockpb.ReservationResponse, error) {
	return s.client.GetReservation(ctx, &stockpb.ReservationKey{ReservationId: id, CustomerId: customer})
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
