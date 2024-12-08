package query

import (
	"context"
	"github.com/jiahuipaung/gorder/stock/entity"
	"github.com/jiahuipaung/gorder/stock/infrastructure/integration"

	"github.com/jiahuipaung/gorder/common/decorator"
	domain "github.com/jiahuipaung/gorder/stock/domain/stock"
	"github.com/sirupsen/logrus"
)

type CheckIfItemsInStock struct {
	Items []*entity.ItemWithQuantity
}

type CheckIfItemsInStockHandler decorator.QueryHandler[CheckIfItemsInStock, []*entity.Item]

type checkIfItemsInStockHandler struct {
	stockRepo domain.Repository
	stripeAPI *integration.StripeAPI
}

func (h checkIfItemsInStockHandler) Handler(ctx context.Context, query CheckIfItemsInStock) ([]*entity.Item, error) {
	if err := h.checkStock(ctx, query.Items); err != nil {
		return nil, err
	}
	var res []*entity.Item
	for _, item := range query.Items {
		//从stripe获取 PriceID
		priceID, err := h.stripeAPI.GetPriceByProductID(ctx, item.ID)
		if err != nil {
			logrus.Warnf("GetPriceByProductID error: %v, item.ID=%s", err, item.ID)
			continue
		}
		res = append(res, &entity.Item{
			ID:       item.ID,
			Quantity: item.Quantity,
			PriceID:  priceID,
		})
	}
	// TODO:扣库存

	return res, nil
}

func (h checkIfItemsInStockHandler) checkStock(ctx context.Context, query []*entity.ItemWithQuantity) error {
	var ids []string
	for _, item := range query {
		ids = append(ids, item.ID)
	}
	records, err := h.stockRepo.GetStock(ctx, ids)
	if err != nil {
		return err
	}
	var idQuantityMap = make(map[string]int32)
	for _, item := range records {
		idQuantityMap[item.ID] = item.Quantity
	}
	var (
		ok       = true
		failedOn []struct {
			ID   string
			Want int32
			Have int32
		}
	)
	for _, item := range query {
		if item.Quantity > idQuantityMap[item.ID] {
			ok = false
			failedOn = append(failedOn, struct {
				ID   string
				Want int32
				Have int32
			}{ID: item.ID, Want: item.Quantity, Have: idQuantityMap[item.ID]})
		}
	}
	if ok {
		return nil
	}
	return domain.ExceedStockError{FailedOn: failedOn}

}

func NewCheckIfItemsInStockHandler(
	stockRepo domain.Repository,
	stripeAPI *integration.StripeAPI,
	logger *logrus.Entry,
	metricsClient decorator.MetricsClient,
) CheckIfItemsInStockHandler {
	if stockRepo == nil {
		panic("stockRepo is nil")
	}
	if stripeAPI == nil {
		panic("stripeAPI is nil")
	}
	return decorator.ApplyQueryDecorators[CheckIfItemsInStock, []*entity.Item](
		checkIfItemsInStockHandler{
			stockRepo: stockRepo,
			stripeAPI: stripeAPI,
		},
		logger,
		metricsClient,
	)
}
