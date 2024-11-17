package service

import (
	"context"
	"github.com/jiahuipaung/gorder/common/broker"
	grpc_client "github.com/jiahuipaung/gorder/common/client"
	"github.com/jiahuipaung/gorder/common/metrics"
	"github.com/jiahuipaung/gorder/order/adapters"
	"github.com/jiahuipaung/gorder/order/adapters/grpc"
	"github.com/jiahuipaung/gorder/order/app"
	"github.com/jiahuipaung/gorder/order/app/command"
	"github.com/jiahuipaung/gorder/order/app/query"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func NewApplication(ctx context.Context) (app.Application, func()) {
	stockClient, closeStockClient, err := grpc_client.NewStockGrpcClient(ctx)
	if err != nil {
		panic(err)
	}
	ch, connectClose := broker.Connect(
		viper.Sub("rabbit-mq").GetString("user"),
		viper.Sub("rabbit-mq").GetString("password"),
		viper.Sub("rabbit-mq").GetString("host"),
		viper.Sub("rabbit-mq").GetString("port"),
	)
	stockGrpc := grpc.NewStockGrpc(stockClient)
	return newApplication(ctx, stockGrpc, ch), func() {
		_ = closeStockClient()
		_ = ch.Close()
		_ = connectClose()
	}
}

func newApplication(_ context.Context, stockGrpc query.StockService, channel *amqp.Channel) app.Application {
	orderRepo := adapters.NewMemoryOrderRepository()
	logger := logrus.NewEntry(logrus.StandardLogger())
	metricsClient := metrics.TodoMetrics{}
	return app.Application{
		Commands: app.Commands{
			CreateOrder: command.NewCreateOrderHandler(
				orderRepo,
				stockGrpc,
				logger,
				channel,
				metricsClient,
			),
			UpdateOrder: command.NewUpdateOrderHandler(
				orderRepo,
				logger,
				metricsClient,
			),
		},
		Queries: app.Queries{
			GetCustomerOrder: query.NewGetCustomerOrderHandler(
				orderRepo,
				logger,
				metricsClient,
			),
		},
	}
}
