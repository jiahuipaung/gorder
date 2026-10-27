package query

import (
	"context"
	"errors"
	"fmt"
	"github.com/jiahuipaung/gorder/stock/entity"
	"math"
	"sort"
	"strings"

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
	stripeAPI PriceLookup
}

type PriceLookup interface {
	GetPriceByProductID(context.Context, string) (string, error)
}

func (h checkIfItemsInStockHandler) Handler(ctx context.Context, query CheckIfItemsInStock) ([]*entity.Item, error) {
	items, err := normalizeItems(query.Items)
	if err != nil {
		return nil, err
	}
	query.Items = items
	if err := h.checkStock(ctx, query.Items); err != nil {
		return nil, err
	}
	var res []*entity.Item
	for _, item := range query.Items {
		//从stripe获取 PriceID
		priceID, err := h.stripeAPI.GetPriceByProductID(ctx, item.ID)
		if err != nil {
			return nil, fmt.Errorf("price lookup for %s: %w", item.ID, err)
		}
		if priceID == "" {
			return nil, fmt.Errorf("product %s has no price", item.ID)
		}
		res = append(res, &entity.Item{
			ID:       item.ID,
			Quantity: item.Quantity,
			PriceID:  priceID,
		})
	}
	// This is a preflight read. Reserve is the authoritative stock deduction.

	return res, nil
}

func normalizeItems(items []*entity.ItemWithQuantity) ([]*entity.ItemWithQuantity, error) {
	if len(items) == 0 {
		return nil, errors.New("cart is empty")
	}
	quantities := make(map[string]int32)
	for _, item := range items {
		if item == nil || strings.TrimSpace(item.ID) == "" || item.Quantity <= 0 {
			return nil, errors.New("SKU and positive quantity are required")
		}
		if int64(quantities[item.ID])+int64(item.Quantity) > math.MaxInt32 {
			return nil, errors.New("item quantity exceeds supported limit")
		}
		quantities[item.ID] += item.Quantity
	}
	result := make([]*entity.ItemWithQuantity, 0, len(quantities))
	for id, quantity := range quantities {
		result = append(result, &entity.ItemWithQuantity{ID: id, Quantity: quantity})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
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
	stripeAPI PriceLookup,
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
