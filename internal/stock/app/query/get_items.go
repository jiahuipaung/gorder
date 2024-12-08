package query

import (
	"context"
	"github.com/jiahuipaung/gorder/stock/entity"

	"github.com/jiahuipaung/gorder/common/decorator"
	domain "github.com/jiahuipaung/gorder/stock/domain/stock"
	"github.com/sirupsen/logrus"
)

type GetItems struct {
	ItemsID []string
}

type GetItemsHandler decorator.QueryHandler[GetItems, []*entity.Item]

type getItemsHandler struct {
	stockRepo domain.Repository
}

func (g getItemsHandler) Handler(ctx context.Context, query GetItems) ([]*entity.Item, error) {
	items, err := g.stockRepo.GetItems(ctx, query.ItemsID)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func NewGetItemsHandler(
	stockRepo domain.Repository,
	logger *logrus.Entry,
	metricClient decorator.MetricsClient,
) GetItemsHandler {
	if stockRepo == nil {
		panic("stockRepo is nil")
	}
	return decorator.ApplyQueryDecorators(
		getItemsHandler{stockRepo: stockRepo},
		logger,
		metricClient,
	)
}
