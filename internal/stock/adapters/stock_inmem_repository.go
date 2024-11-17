package adapters

import (
	"context"
	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	domain "github.com/jiahuipaung/gorder/stock/domain/stock"
	"sync"
)

type MemoryStockRepository struct {
	lock  *sync.RWMutex
	store map[string]*orderpb.Item
}

var stub = map[string]*orderpb.Item{
	"item_id": {
		ID:       "foo_item",
		Name:     "stub item",
		Quantity: 1000,
		PriceID:  "stub_item_price_id",
	},
	"test-1": {
		ID:       "foo_test_1",
		Name:     "stub item 1",
		Quantity: 1000,
		PriceID:  "stub_item1_price_id",
	},
	"test-2": {
		ID:       "foo_test_2",
		Name:     "stub item 2",
		Quantity: 1000,
		PriceID:  "stub_item2_price_id",
	},
	"test-3": {
		ID:       "foo_test_3",
		Name:     "stub item 3",
		Quantity: 1000,
		PriceID:  "stub_item3_price_id",
	},
}

func NewMemoryStockRepository() *MemoryStockRepository {
	return &MemoryStockRepository{
		lock:  &sync.RWMutex{},
		store: stub,
	}
}

func (m *MemoryStockRepository) GetItems(ctx context.Context, ids []string) ([]*orderpb.Item, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	var (
		res     []*orderpb.Item
		missing []string
	)
	for _, id := range ids {
		if item, ok := m.store[id]; ok {
			res = append(res, item)
		} else {
			missing = append(missing, id)
		}
	}
	if len(res) == len(ids) {
		return res, nil
	}

	return res, domain.NotFoundError{Missing: missing}
}
