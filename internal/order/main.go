package main

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/jiahuipaung/gorder/common/broker"
	"github.com/jiahuipaung/gorder/common/config"
	"github.com/jiahuipaung/gorder/common/discovery"
	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/common/logging"
	"github.com/jiahuipaung/gorder/common/server"
	"github.com/jiahuipaung/gorder/common/tracing"
	"github.com/jiahuipaung/gorder/order/infrastructure/consumer"
	"github.com/jiahuipaung/gorder/order/ports"
	"github.com/jiahuipaung/gorder/order/service"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

func init() {
	if err := config.NewViperConfig(); err != nil {
		logrus.Fatal(err)
	}
}

func main() {
	logging.Init()
	serviceName := viper.Sub("order").GetString("service-name")

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

	deRegisterFunc, err := discovery.RegisterToConsul(ctx, serviceName)
	if err != nil {
		logrus.Fatal(err)
	}
	defer func() {
		_ = deRegisterFunc()
	}()

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

	go server.RunGRPCServer(serviceName, func(s *grpc.Server) {
		svc := ports.NewGRPCServer(application)
		orderpb.RegisterOrderServiceServer(s, svc)
	})

	server.RunHTTPServer(serviceName, func(router *gin.Engine) {
		router.StaticFile("/success", "../../public/success.html")
		ports.RegisterHandlersWithOptions(router, &HTTPServer{
			app: application,
		}, ports.GinServerOptions{
			BaseURL:      "/api",
			Middlewares:  nil,
			ErrorHandler: nil,
		})
	})

}
