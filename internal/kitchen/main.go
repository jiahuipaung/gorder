package main

import (
	"context"
	"github.com/jiahuipaung/gorder/common/broker"
	grpcClient "github.com/jiahuipaung/gorder/common/client"
	_ "github.com/jiahuipaung/gorder/common/config"
	"github.com/jiahuipaung/gorder/common/logging"
	"github.com/jiahuipaung/gorder/common/tracing"
	"github.com/jiahuipaung/gorder/kitchen/adapter"
	"github.com/jiahuipaung/gorder/kitchen/infrastructure/consumer"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"os"
	"os/signal"
	"syscall"
)

func init() {
	logging.Init()
}

func main() {
	serviceName := viper.Sub("kitchen").GetString("service-name")

	ctx, cancel := context.WithCancel(context.Background()) // 用于超时退出
	defer cancel()

	shutdown, err := tracing.InitJaegerProvider(viper.Sub("jaeger").GetString("url"), serviceName)
	if err != nil {
		logrus.Fatal(err)
	}
	defer func() {
		_ = shutdown(ctx)
	}()

	orderClient, closeFunc, err := grpcClient.NewOrderGrpcClient(ctx)
	if err != nil {
		logrus.Fatal(err)
	}
	defer func() {
		_ = closeFunc()
	}()
	orderGPRC := adapter.OrderGRPC{Client: orderClient}
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
	go consumer.NewConsumer(&orderGPRC).Listen(ch)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		logrus.Info("Exit signal received")
		os.Exit(0)
	}()
	logrus.Println("Press Ctrl+C  to exit")
	select {}
}
