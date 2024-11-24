package ports

import (
	"context"
	"github.com/jiahuipaung/gorder/common/tracing"

	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
	"github.com/jiahuipaung/gorder/stock/app"
	"github.com/jiahuipaung/gorder/stock/app/query"
)

type GRPCServer struct {
	app app.Application
}

func NewGRPCServer(app app.Application) *GRPCServer {
	return &GRPCServer{app: app}
}

func (G GRPCServer) GetItems(ctx context.Context, request *stockpb.GetItemsRequest) (*stockpb.GetItemsResponse, error) {
	_, span := tracing.Start(ctx, "GRPC.GetItems")
	defer span.End()
	items, err := G.app.Queries.GetItems.Handler(ctx, query.GetItems{ItemsID: request.ItemIDs})
	if err != nil {
		return nil, err
	}
	return &stockpb.GetItemsResponse{Items: items}, nil
}

func (G GRPCServer) CheckIfItemsInStock(ctx context.Context, request *stockpb.CheckIfItemsInStockRequest) (*stockpb.CheckIfItemsInStockResponse, error) {
	_, span := tracing.Start(ctx, "GRPC.CheckIfItemsInStock")
	defer span.End()
	items, err := G.app.Queries.CheckIfItemsInStock.Handler(ctx, query.CheckIfItemsInStock{Items: request.Items})
	if err != nil {
		return nil, err
	}
	return &stockpb.CheckIfItemsInStockResponse{InStock: 1, Items: items}, nil
}
