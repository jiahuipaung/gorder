package service

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"time"

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
	//orderRepo := adapters.NewMemoryOrderRepository()
	mongoClient := newMongoClient()
	orderRepo := adapters.NewOrderRepositoryMongo(mongoClient)
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

func newMongoClient() *mongo.Client {
	uri := fmt.Sprintf(
		"mongodb://%s:%s@%s:%s",
		viper.Sub("mongo").GetString("username"),
		viper.Sub("mongo").GetString("password"),
		viper.Sub("mongo").GetString("host"),
		viper.Sub("mongo").GetString("port"),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	connect, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		panic(err)
	}
	if err := connect.Ping(ctx, readpref.Primary()); err != nil {
		panic(err)
	}
	return connect
}
