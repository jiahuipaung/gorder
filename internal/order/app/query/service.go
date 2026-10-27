package query

import (
	"context"

	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
)

type StockService interface {
	CheckIfItemsInStock(ctx context.Context, items []*orderpb.ItemWithQuantity) (*stockpb.CheckIfItemsInStockResponse, error)
	Reserve(ctx context.Context, orderID, customerID string, items []*orderpb.ItemWithQuantity, expiresAtUnixMillis int64) (*stockpb.ReservationResponse, error)
	GetReservation(ctx context.Context, orderID, customerID string) (*stockpb.ReservationResponse, error)
	GetItem(ctx context.Context, itemID []string) ([]*orderpb.Item, error)
}
