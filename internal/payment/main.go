package main

import (
	"context"
	"github.com/jiahuipaung/gorder/common/broker"
	"github.com/jiahuipaung/gorder/common/config"
	"github.com/jiahuipaung/gorder/common/logging"
	"github.com/jiahuipaung/gorder/common/server"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"payment/infrastructure/consumer"
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

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	go consumer.NewConsumer().Listen(ch)
	paymentHandler := NewPaymentHandler()

	switch serviceType {
	case "http":
		server.RunHTTPServer(serviceName, paymentHandler.RegisterRoutes)
	case "grpc":
		logrus.Panic("unexpected gRPC service")
	default:
		logrus.Panic("unexpected service type: " + serviceType)
	}

}
