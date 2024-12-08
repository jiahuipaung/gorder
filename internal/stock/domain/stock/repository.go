package stock

import (
	"context"
	"fmt"
	"github.com/jiahuipaung/gorder/stock/entity"
	"strings"
)

type Repository interface {
	GetItems(ctx context.Context, ids []string) ([]*entity.Item, error)
	GetStock(ctx context.Context, ids []string) ([]*entity.ItemWithQuantity, error)
}

type NotFoundError struct {
	Missing []string
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("these items with id %s not found in stock", strings.Join(e.Missing, ","))
}

type ExceedStockError struct {
	FailedOn []struct {
		ID   string
		Want int32
		Have int32
	}
}

func (e ExceedStockError) Error() string {
	var info []string
	for _, item := range e.FailedOn {
		info = append(info, fmt.Sprintf("product_id=%s, want=%s, have=%s", item.ID, item.Want, item.Have))
	}
	return fmt.Sprintf("not enough stock for [%s]", strings.Join(info, ","))
}
