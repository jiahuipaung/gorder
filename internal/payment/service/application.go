package service

import (
	"context"
	"payment/infrastructure/processor"

	grpc_client "github.com/jiahuipaung/gorder/common/client"
	"github.com/jiahuipaung/gorder/common/metrics"
	"github.com/jiahuipaung/gorder/payment/adapters"
	"github.com/jiahuipaung/gorder/payment/app"
	"github.com/jiahuipaung/gorder/payment/app/command"
	"github.com/jiahuipaung/gorder/payment/domain"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func NewApplication(ctx context.Context) (app.Application, func()) {
	orderClient, closeOrderClient, err := grpc_client.NewOrderGrpcClient(ctx)
	if err != nil {
		panic(err)
	}
	orderGrpc := adapters.NewOrderGrpc(orderClient)
	//memProcessor := processor.NewInMemProcessor()
	stripeProcessor := processor.NewStripePrcessor(viper.GetString("stripe-key"))
	return newApplication(ctx, orderGrpc, stripeProcessor), func() {
		_ = closeOrderClient()
	}
}

func newApplication(_ context.Context, orderGrpc command.OrderService, processor domain.Processor) app.Application {
	logger := logrus.NewEntry(logrus.StandardLogger())
	metricsClient := metrics.TodoMetrics{}

	return app.Application{
		Commands: app.Commands{
			CreatePayment: command.NewCreatePaymentHandler(
				processor,
				orderGrpc,
				logger,
				metricsClient,
			),
		},
	}
}
