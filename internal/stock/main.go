package main

import (
	"context"
	"github.com/jiahuipaung/gorder/common/tracing"

	"github.com/jiahuipaung/gorder/common/config"
	"github.com/jiahuipaung/gorder/common/discovery"
	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
	"github.com/jiahuipaung/gorder/common/logging"
	"github.com/jiahuipaung/gorder/common/server"
	"github.com/jiahuipaung/gorder/stock/ports"
	"github.com/jiahuipaung/gorder/stock/service"
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
	serviceName := viper.Sub("stock").GetString("service-name")
	serviceType := viper.Sub("stock").GetString("server-to-run")

	ctx, cancel := context.WithCancel(context.Background()) // 用于超时退出
	defer cancel()

	shutdown, err := tracing.InitJaegerProvider(viper.Sub("jaeger").GetString("url"), serviceName)
	if err != nil {
		logrus.Fatal(err)
	}
	defer func() {
		_ = shutdown(ctx)
	}()

	application := service.NewApplication(ctx)
	deRegisterFunc, err := discovery.RegisterToConsul(ctx, serviceName)
	if err != nil {
		logrus.Fatal(err)
	}
	defer func() {
		_ = deRegisterFunc()
	}()

	switch serviceType {
	case "grpc":
		server.RunGRPCServer(serviceName, func(s *grpc.Server) {
			svc := ports.NewGRPCServer(application)
			stockpb.RegisterStockServiceServer(s, svc)
		})
	case "http":
		// TODO:
		//server.RunHTTPServer(serviceName, func(s *http.Server) {})
	default:
		panic("unexpected service type: " + serviceType)
	}

}
