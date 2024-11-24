package stock

import (
	"context"
	"fmt"
	"strings"

	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
)

type Repository interface {
	GetItems(ctx context.Context, ids []string) ([]*orderpb.Item, error)
}

type NotFoundError struct {
	Missing []string
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("these items with id %s not found in stock", strings.Join(e.Missing, ","))
}
