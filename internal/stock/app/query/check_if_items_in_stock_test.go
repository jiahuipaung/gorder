package query

import (
	"context"
	"errors"
	"github.com/jiahuipaung/gorder/stock/entity"
	"testing"
)

type catalog struct{}

func (catalog) GetItems(context.Context, []string) ([]*entity.Item, error) { return nil, nil }
func (catalog) GetStock(context.Context, []string) ([]*entity.ItemWithQuantity, error) {
	return []*entity.ItemWithQuantity{{ID: "a", Quantity: 10}, {ID: "b", Quantity: 10}}, nil
}

type prices struct {
	failed string
	empty  bool
}

func (p prices) GetPriceByProductID(_ context.Context, id string) (string, error) {
	if id == p.failed {
		return "", errors.New("price unavailable")
	}
	if p.empty {
		return "", nil
	}
	return "price_" + id, nil
}

func TestPriceFailureRejectsWholeCart(t *testing.T) {
	for _, p := range []prices{{failed: "b"}, {empty: true}} {
		h := checkIfItemsInStockHandler{stockRepo: catalog{}, stripeAPI: p}
		got, err := h.Handler(context.Background(), CheckIfItemsInStock{Items: []*entity.ItemWithQuantity{{ID: "a", Quantity: 1}, {ID: "b", Quantity: 1}}})
		if err == nil || got != nil {
			t.Fatalf("partial cart returned: %v, %v", got, err)
		}
	}
}

func TestStockRejectsInvalidAndUnaggregatedQuantities(t *testing.T) {
	for _, items := range [][]*entity.ItemWithQuantity{
		nil, {nil}, {{ID: "a", Quantity: -1}}, {{ID: "a", Quantity: 0}}, {{Quantity: 1}},
		{{ID: "a", Quantity: 6}, {ID: "a", Quantity: 6}},
	} {
		h := checkIfItemsInStockHandler{stockRepo: catalog{}, stripeAPI: prices{}}
		if _, err := h.Handler(context.Background(), CheckIfItemsInStock{Items: items}); err == nil {
			t.Fatalf("accepted %v", items)
		}
	}
}
