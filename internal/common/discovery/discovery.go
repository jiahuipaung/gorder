package discovery

import (
	"context"
	"fmt"
	"github.com/google/uuid"
)

type Registry interface {
	Register(ctx context.Context, instanceID, serviceName, hostPort string) error
	Deregister(ctx context.Context, instanceID, serviceName string) error
	Discovery(ctx context.Context, instanceID, serviceName string) ([]string, error)
	HealthCheck(instanceID, serviceName string) error
}

func GenerateInstanceID(serviceName string) string {
	return fmt.Sprintf("%s-%s", serviceName, uuid.New())
}
