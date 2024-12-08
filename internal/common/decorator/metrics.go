package decorator

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type MetricsClient interface {
	Inc(key string, value int)
}

type queryMetricsDecorator[C, R any] struct {
	base   QueryHandler[C, R]
	client MetricsClient
}

func (q queryMetricsDecorator[C, R]) Handler(ctx context.Context, cmd C) (result R, err error) {
	start := time.Now()
	actionName := strings.ToLower(generateActionName(cmd))

	defer func() {
		end := time.Since(start)
		q.client.Inc(fmt.Sprintf("query.%s.duration", actionName), int(end.Seconds()))
		if err == nil {
			q.client.Inc(fmt.Sprintf("query.%s.success", actionName), 1)
		} else {
			q.client.Inc(fmt.Sprintf("query.%s.failure", actionName), 1)
		}
	}()
	return q.base.Handler(ctx, cmd)
}

type commandMetricsDecorator[C, R any] struct {
	base   CommandHandler[C, R]
	client MetricsClient
}

func (c commandMetricsDecorator[C, R]) Handler(ctx context.Context, cmd C) (result R, err error) {
	start := time.Now()
	actionName := strings.ToLower(generateActionName(cmd))

	defer func() {
		end := time.Since(start)
		c.client.Inc(fmt.Sprintf("command.%s.duration", actionName), int(end.Seconds()))
		if err == nil {
			c.client.Inc(fmt.Sprintf("command.%s.success", actionName), 1)
		} else {
			c.client.Inc(fmt.Sprintf("command.%s.failure", actionName), 1)
		}
	}()
	return c.base.Handler(ctx, cmd)
}
