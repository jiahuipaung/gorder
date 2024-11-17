package main

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/jiahuipaung/gorder/common/config"
	"github.com/jiahuipaung/gorder/common/discovery"
	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/common/logging"
	"github.com/jiahuipaung/gorder/common/server"
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

	application, cleanup := service.NewApplication(ctx)
	defer cleanup()

	deRegisterFunc, err := discovery.RegisterToConsul(ctx, serviceName)
	if err != nil {
		logrus.Fatal(err)
	}
	defer func() {
		_ = deRegisterFunc()
	}()

	go server.RunGRPCServer(serviceName, func(s *grpc.Server) {
		svc := ports.NewGRPCServer(application)
		orderpb.RegisterOrderServiceServer(s, svc)
	})

	server.RunHTTPServer(serviceName, func(router *gin.Engine) {
		ports.RegisterHandlersWithOptions(router, &HTTPServer{
			app: application,
		}, ports.GinServerOptions{
			BaseURL:      "/api",
			Middlewares:  nil,
			ErrorHandler: nil,
		})
	})

}
