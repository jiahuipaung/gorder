package client

import (
	"context"
	"fmt"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"net"
	"time"

	"github.com/jiahuipaung/gorder/common/discovery"
	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func NewStockGrpcClient(ctx context.Context) (client stockpb.StockServiceClient, close func() error, err error) {
	if !waitForStockGrpcClient(viper.GetDuration("dial-grpc-timeout") * time.Second) {
		return nil, nil, fmt.Errorf("timeout waiting for stock grpc client")
	}
	grpcAddr, err := discovery.GetServiceAddr(ctx, viper.Sub("stock").GetString("service-name"))
	if err != nil {
		return nil, func() error {
			return nil
		}, err
	}
	if grpcAddr == "" {
		logrus.Warn("stock grpc address is empty")
	}
	opts := grpcDialOpts(grpcAddr)

	newClient, err := grpc.NewClient(grpcAddr, opts...)
	if err != nil {
		return nil, func() error { return nil }, err
	}
	return stockpb.NewStockServiceClient(newClient), newClient.Close, nil
}

func NewOrderGrpcClient(ctx context.Context) (client orderpb.OrderServiceClient, close func() error, err error) {
	if !waitForOrderGrpcClient(viper.GetDuration("dial-grpc-timeout") * time.Second) {
		return nil, nil, fmt.Errorf("timeout waiting for order grpc client")
	}
	grpcAddr, err := discovery.GetServiceAddr(ctx, viper.Sub("order").GetString("service-name"))
	if err != nil {
		return nil, func() error {
			return nil
		}, err
	}
	if grpcAddr == "" {
		logrus.Warn("order grpc address is empty")
	}
	opts := grpcDialOpts(grpcAddr)

	newClient, err := grpc.NewClient(grpcAddr, opts...)
	if err != nil {
		return nil, func() error { return nil }, err
	}
	return orderpb.NewOrderServiceClient(newClient), newClient.Close, nil
}

func grpcDialOpts(_ string) []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
}

func waitForOrderGrpcClient(timeout time.Duration) bool {
	logrus.Infof("waiting for order grpc client connection timeout %v seconds", timeout.Seconds())
	return waitFor(viper.Sub("order").GetString("grpc-addr"), timeout)
}

func waitForStockGrpcClient(timeout time.Duration) bool {
	logrus.Infof("waiting for stock grpc client connection timeout %v seconds", timeout.Seconds())
	return waitFor(viper.Sub("stock").GetString("grpc-addr"), timeout)
}

func waitFor(addr string, timeout time.Duration) bool {
	portAvailable := make(chan struct{})
	timeoutCh := time.After(timeout)

	go func() {
		for {
			select {
			case <-timeoutCh:
				return
			default:
				//continue
			}
			_, err := net.Dial("tcp", addr)
			if err == nil {
				close(portAvailable)
				return
			}
			time.Sleep(200 * time.Millisecond)

		}
	}()

	select {
	case <-portAvailable:
		return true
	case <-timeoutCh:
		return false
	}
}
