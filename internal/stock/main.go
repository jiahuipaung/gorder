package main

import (
	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
	"github.com/jiahuipaung/gorder/common/server"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"stock/ports"
)

func main() {
	serviceName := viper.GetString("stock.service-name")
	serviceType := viper.GetString("stock.server-to-run")
	switch serviceType {
	case "grpc":
		server.RunGRPCServer(serviceName, func(s *grpc.Server) {
			svc := ports.NewGRPCServer()
			stockpb.RegisterStockServiceServer(s, svc)
		})
	case "http":
		// TODO:
		//server.RunHTTPServer(serviceName, func(s *http.Server) {})
	default:
		panic("unexpected service type: " + serviceType)
	}

}
