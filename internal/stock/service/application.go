package service

import (
	"context"
	"github.com/jiahuipaung/gorder/common/metrics"
	"github.com/jiahuipaung/gorder/stock/adapters"
	"github.com/jiahuipaung/gorder/stock/app"
	"github.com/jiahuipaung/gorder/stock/app/query"
	"github.com/jiahuipaung/gorder/stock/infrastructure/integration"
	"github.com/jiahuipaung/gorder/stock/infrastructure/persistent"
	"github.com/jiahuipaung/gorder/stock/reservation"
	"github.com/sirupsen/logrus"
)

func NewApplication(ctx context.Context) app.Application {
	//stockRepo := adapters.NewMemoryStockRepository()
	db := persistent.NewMySQL()
	reservations := reservation.NewStore(db.DB())
	if err := reservations.Migrate(ctx); err != nil {
		logrus.Panicf("inventory reservation schema unavailable: %v", err)
	}
	stockRepo := adapters.NewMySQLStockRepository(db)
	stripeAPI := integration.NewStripeAPI()
	logger := logrus.NewEntry(logrus.StandardLogger())
	metricsClient := metrics.TodoMetrics{}
	return app.Application{
		Reservations: reservations,

		Commands: app.Commands{},
		Queries: app.Queries{
			CheckIfItemsInStock: query.NewCheckIfItemsInStockHandler(
				stockRepo,
				stripeAPI,
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
