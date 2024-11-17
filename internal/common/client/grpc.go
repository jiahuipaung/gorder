package client

import (
	"context"
	"github.com/jiahuipaung/gorder/common/discovery"
	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func NewStockGrpcClient(ctx context.Context) (client stockpb.StockServiceClient, close func() error, err error) {
	grpcAddr, err := discovery.GetServiceAddr(ctx, viper.Sub("stock").GetString("service-name"))
	if err != nil {
		return nil, func() error {
			return nil
		}, err
	}
	if grpcAddr == "" {
		logrus.Warn("stock grpc address is empty")
	}
	opts, err := grpcDialOpts(grpcAddr)
	if err != nil {
		return nil, func() error { return nil }, err
	}
	newClient, err := grpc.NewClient(grpcAddr, opts...)
	if err != nil {
		return nil, func() error { return nil }, err
	}
	return stockpb.NewStockServiceClient(newClient), newClient.Close, nil
}

func grpcDialOpts(addr string) ([]grpc.DialOption, error) {
	return []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}, nil
}
