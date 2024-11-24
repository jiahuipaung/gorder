package main

import (
	"context"
	"github.com/jiahuipaung/gorder/common/tracing"

	"github.com/jiahuipaung/gorder/common/broker"
	"github.com/jiahuipaung/gorder/common/config"
	"github.com/jiahuipaung/gorder/common/logging"
	"github.com/jiahuipaung/gorder/common/server"
	"github.com/jiahuipaung/gorder/payment/infrastructure/consumer"
	"github.com/jiahuipaung/gorder/payment/service"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func init() {
	if err := config.NewViperConfig(); err != nil {
		logrus.Fatal(err)
	}
}

func main() {
	logging.Init()
	serviceName := viper.Sub("payment").GetString("service-name")
	serviceType := viper.Sub("payment").GetString("server-to-run")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdown, err := tracing.InitJaegerProvider(viper.Sub("jaeger").GetString("url"), serviceName)
	if err != nil {
		logrus.Fatal(err)
	}
	defer func() {
		_ = shutdown(ctx)
	}()

	application, cleanup := service.NewApplication(ctx)
	defer cleanup()

	ch, connectClose := broker.Connect(
		viper.Sub("rabbit-mq").GetString("user"),
		viper.Sub("rabbit-mq").GetString("password"),
		viper.Sub("rabbit-mq").GetString("host"),
		viper.Sub("rabbit-mq").GetString("port"),
	)
	defer func() {
		_ = ch.Close()
		_ = connectClose()
	}()

	go consumer.NewConsumer(application).Listen(ch)
	paymentHandler := NewPaymentHandler(ch)

	switch serviceType {
	case "http":
		server.RunHTTPServer(serviceName, paymentHandler.RegisterRoutes)
	case "grpc":
		logrus.Panic("unexpected gRPC service")
	default:
		logrus.Panic("unexpected service type: " + serviceType)
	}

}
