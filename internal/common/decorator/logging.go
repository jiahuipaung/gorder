package decorator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

type queryLoggingDecorator[C, R any] struct {
	logger *logrus.Entry
	base   QueryHandler[C, R]
}

func (q queryLoggingDecorator[C, R]) Handler(ctx context.Context, cmd C) (result R, err error) {
	body, _ := json.Marshal(cmd)
	logger := q.logger.WithFields(logrus.Fields{
		"query":      generateActionName(cmd),
		"query_body": string(body),
	})
	logger.Debug("Executing query")
	defer func() {
		if err == nil {
			logger.Debug("Query succeeded")
		} else {
			logger.Error("Query failed", err)
		}
	}()
	result, err = q.base.Handler(ctx, cmd)
	return
}

type commandLoggingDecorator[C, R any] struct {
	logger *logrus.Entry
	base   CommandHandler[C, R]
}

func (c commandLoggingDecorator[C, R]) Handler(ctx context.Context, cmd C) (result R, err error) {
	body, _ := json.Marshal(cmd)
	logger := c.logger.WithFields(logrus.Fields{
		"command":      generateActionName(cmd),
		"command_body": string(body),
	})
	logger.Debug("Executing command")
	defer func() {
		if err == nil {
			logger.Debug("Command succeeded")
		} else {
			logger.Error("Command failed", err)
		}
	}()
	result, err = c.base.Handler(ctx, cmd)
	return
}

func generateActionName(cmd any) string {
	return strings.Split(fmt.Sprintf("%T", cmd), ".")[1]
}
