package discovery

import (
	"context"
	"fmt"
	"github.com/jiahuipaung/gorder/common/discovery/consul"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"math/rand"
	"time"
)

func RegisterToConsul(ctx context.Context, serviceName string) (func() error, error) {
	register, err := consul.New(viper.Sub("consul").GetString("addr"))
	if err != nil {
		return func() error {
			return nil
		}, err
	}
	instanceID := GenerateInstanceID(serviceName)
	grpcAddr := viper.Sub(serviceName).GetString("grpc-addr")
	if err := register.Register(ctx, instanceID, serviceName, grpcAddr); err != nil {
		return func() error {
			return nil
		}, err
	}

	go func() {
		for {
			if err := register.HealthCheck(instanceID, serviceName); err != nil {
				logrus.Panic("No heartbeat from %s to registry, err=%v", serviceName, err)
			}
			time.Sleep(1 * time.Second)
		}
	}()
	logrus.WithFields(
		logrus.Fields{
			"serviceName": serviceName,
			"instanceID":  instanceID,
			"grpcAddr":    grpcAddr,
		}).Infof("register to consul")
	return func() error { // cleanup
		return register.Deregister(ctx, instanceID, serviceName)
	}, nil
}

func GetServiceAddr(ctx context.Context, serviceName string) (string, error) {
	registry, err := consul.New(viper.Sub("consul").GetString("addr"))
	if err != nil {
		return "", err
	}
	addrs, err := registry.Discover(ctx, serviceName)
	if err != nil {
		return "", err
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf(" get empty addrs %s from consul", serviceName)
	}
	i := rand.Intn(len(addrs))
	logrus.Infof("discover %d instances of %s, addrs=%v", len(addrs), serviceName, addrs)
	return addrs[i], nil
}
