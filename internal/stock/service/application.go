package service

import (
	"context"

	"github.com/jiahuipaung/gorder/common/metrics"
	"github.com/jiahuipaung/gorder/stock/adapters"
	"github.com/jiahuipaung/gorder/stock/app"
	"github.com/jiahuipaung/gorder/stock/app/query"
	"github.com/sirupsen/logrus"
)

func NewApplication(_ context.Context) app.Application {
	stockRepo := adapters.NewMemoryStockRepository()
	logger := logrus.NewEntry(logrus.StandardLogger())
	metricsClient := metrics.TodoMetrics{}
	return app.Application{

		Commands: app.Commands{},
		Queries: app.Queries{
			CheckIfItemsInStock: query.NewCheckIfItemsInStockHandler(
				stockRepo,
				logger,
				metricsClient,
			),
			GetItems: query.NewGetItemsHandler(
				stockRepo,
				logger,
				metricsClient,
			),
		},
	}
}
