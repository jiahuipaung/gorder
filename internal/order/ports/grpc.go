package ports

import (
	"context"
	"errors"
	"order/convertor"

	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/order/app"
	"github.com/jiahuipaung/gorder/order/app/command"
	"github.com/jiahuipaung/gorder/order/app/query"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type GRPCServer struct {
	app app.Application
}

func NewGRPCServer(app app.Application) *GRPCServer {
	return &GRPCServer{app: app}
}

func (G GRPCServer) CreateOrder(ctx context.Context, request *orderpb.CreateOrderRequest) (*emptypb.Empty, error) {
	_, err := G.app.Commands.CreateOrder.Handler(ctx, command.CreateOrder{
		CustomerID: request.CustomerID,
		Items:      convertor.NewItemWithQuantityConverter().ProtoToEntities(request.Items),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &emptypb.Empty{}, nil

}

func (G GRPCServer) GetOrder(ctx context.Context, request *orderpb.GetOrderRequest) (*orderpb.Order, error) {
	o, err := G.app.Queries.GetCustomerOrder.Handler(ctx, query.GetCustomerOrder{
		CustomerId: request.CustomerID,
		OrderId:    request.OrderID,
	})
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	return convertor.NewOrderConverter().EntityToProto(o), nil
}

func (G GRPCServer) AttachPaymentSession(
	ctx context.Context,
	request *orderpb.AttachPaymentSessionRequest,
) (*emptypb.Empty, error) {
	logrus.Infof("order_grpc.AttachPaymentSession request=%+v", request)
	if request == nil || request.OrderID == "" || request.CustomerID == "" || request.PaymentLink == "" {
		return nil, status.Error(codes.InvalidArgument, "order ID, customer ID, and payment link are required")
	}
	_, err := G.app.Commands.AttachPaymentSession.Handler(ctx, command.AttachPaymentSession{
		OrderID:     request.OrderID,
		CustomerID:  request.CustomerID,
		PaymentLink: request.PaymentLink,
	})
	if err != nil {
		return nil, orderCommandStatus(err)
	}
	return &emptypb.Empty{}, nil
}

func (G GRPCServer) MarkOrderReady(
	ctx context.Context,
	request *orderpb.MarkOrderReadyRequest,
) (*emptypb.Empty, error) {
	logrus.Infof("order_grpc.MarkOrderReady request=%+v", request)
	if request == nil || request.OrderID == "" || request.CustomerID == "" {
		return nil, status.Error(codes.InvalidArgument, "order ID and customer ID are required")
	}
	_, err := G.app.Commands.MarkOrderReady.Handler(ctx, command.MarkOrderReady{
		OrderID:    request.OrderID,
		CustomerID: request.CustomerID,
	})
	if err != nil {
		return nil, orderCommandStatus(err)
	}
	return &emptypb.Empty{}, nil
}

func orderCommandStatus(err error) error {
	var notFound domain.NotFoundError
	if errors.As(err, &notFound) {
		return status.Error(codes.NotFound, err.Error())
	}
	var invalidTransition domain.InvalidStatusTransitionError
	if errors.As(err, &invalidTransition) {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}
