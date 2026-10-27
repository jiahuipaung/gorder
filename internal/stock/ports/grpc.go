package ports

import (
	"context"
	"errors"
	"github.com/jiahuipaung/gorder/common/tracing"
	"github.com/jiahuipaung/gorder/stock/convertor"
	"time"

	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
	"github.com/jiahuipaung/gorder/stock/app"
	"github.com/jiahuipaung/gorder/stock/app/query"
	"github.com/jiahuipaung/gorder/stock/reservation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GRPCServer struct {
	app app.Application
}

func (G GRPCServer) Reserve(ctx context.Context, request *stockpb.ReserveRequest) (*stockpb.ReservationResponse, error) {
	if request == nil || request.ReservationId == "" || request.CustomerId == "" || request.ExpiresAtUnixMillis <= 0 || len(request.Items) == 0 {
		return nil, status.Error(codes.InvalidArgument, "reservation identity, expiry and items are required")
	}
	items := make([]reservation.Item, 0, len(request.Items))
	for _, item := range request.Items {
		if item == nil || item.ID == "" || item.Quantity <= 0 {
			return nil, status.Error(codes.InvalidArgument, "invalid reservation item")
		}
		items = append(items, reservation.Item{SKU: item.ID, Quantity: item.Quantity})
	}
	r, err := G.app.Reservations.Reserve(ctx, reservation.Request{ID: request.ReservationId, CustomerID: request.CustomerId, Items: items, ExpiresAt: time.UnixMilli(request.ExpiresAtUnixMillis)})
	if err != nil {
		return nil, reservationError(err)
	}
	return reservationResponse(r), nil
}

func (G GRPCServer) Confirm(ctx context.Context, request *stockpb.ReservationKey) (*stockpb.ReservationResponse, error) {
	if request == nil || request.ReservationId == "" || request.CustomerId == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation identity required")
	}
	r, err := G.app.Reservations.Confirm(ctx, request.ReservationId, request.CustomerId)
	if err != nil {
		return nil, reservationError(err)
	}
	return reservationResponse(r), nil
}

func (G GRPCServer) Release(ctx context.Context, request *stockpb.ReservationKey) (*stockpb.ReservationResponse, error) {
	if request == nil || request.ReservationId == "" || request.CustomerId == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation identity required")
	}
	r, err := G.app.Reservations.Release(ctx, request.ReservationId, request.CustomerId)
	if err != nil {
		return nil, reservationError(err)
	}
	return reservationResponse(r), nil
}

func (G GRPCServer) GetReservation(ctx context.Context, request *stockpb.ReservationKey) (*stockpb.ReservationResponse, error) {
	if request == nil || request.ReservationId == "" || request.CustomerId == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation identity required")
	}
	r, err := G.app.Reservations.Get(ctx, request.ReservationId, request.CustomerId)
	if err != nil {
		return nil, reservationError(err)
	}
	return reservationResponse(r), nil
}

func reservationResponse(r *reservation.Reservation) *stockpb.ReservationResponse {
	return &stockpb.ReservationResponse{ReservationId: r.ID, CustomerId: r.CustomerID, Status: r.Status, ExpiresAtUnixMillis: r.ExpiresAt.UnixMilli()}
}

func reservationError(err error) error {
	switch {
	case errors.Is(err, reservation.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, reservation.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, reservation.ErrConflict), errors.Is(err, reservation.ErrTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, reservation.ErrInsufficient):
		return status.Error(codes.ResourceExhausted, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
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
	return &stockpb.GetItemsResponse{Items: convertor.NewItemConvertor().EntitiesToProto(items)}, nil
}

func (G GRPCServer) CheckIfItemsInStock(ctx context.Context, request *stockpb.CheckIfItemsInStockRequest) (*stockpb.CheckIfItemsInStockResponse, error) {
	_, span := tracing.Start(ctx, "GRPC.CheckIfItemsInStock")
	defer span.End()
	items, err := G.app.Queries.CheckIfItemsInStock.Handler(ctx, query.CheckIfItemsInStock{
		Items: convertor.NewItemWithQuantityConverter().ProtoToEntities(request.Items)})
	if err != nil {
		return nil, err
	}
	return &stockpb.CheckIfItemsInStockResponse{InStock: 1, Items: convertor.NewItemConvertor().EntitiesToProto(items)}, nil
}
