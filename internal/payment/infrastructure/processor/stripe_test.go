package processor

import (
	"encoding/json"
	"testing"

	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
)

func TestNewCheckoutSessionParamsUsesOrderIDAsIdempotencyKey(t *testing.T) {
	t.Parallel()

	params := newCheckoutSessionParams(&orderpb.Order{
		ID:         "order-1",
		CustomerID: "customer-1",
		Status:     "pending",
		Items: []*orderpb.Item{{
			ID:       "item-1",
			PriceID:  "price-1",
			Quantity: 2,
		}},
	})

	if params.IdempotencyKey == nil || *params.IdempotencyKey != "order-1" {
		t.Fatalf("expected idempotency key order-1, got %v", params.IdempotencyKey)
	}
	if params.Metadata["orderID"] != "order-1" || params.Metadata["customerID"] != "customer-1" {
		t.Fatalf("unexpected metadata: %v", params.Metadata)
	}
	if len(params.LineItems) != 1 {
		t.Fatalf("expected one line item, got %d", len(params.LineItems))
	}
	if params.LineItems[0].Price == nil || *params.LineItems[0].Price != "price-1" {
		t.Fatalf("unexpected price: %v", params.LineItems[0].Price)
	}
	if params.LineItems[0].Quantity == nil || *params.LineItems[0].Quantity != 2 {
		t.Fatalf("unexpected quantity: %v", params.LineItems[0].Quantity)
	}
	var eventItems []*orderpb.Item
	if err := json.Unmarshal([]byte(params.Metadata["items"]), &eventItems); err != nil {
		t.Fatal(err)
	}
	if len(eventItems) != 1 || eventItems[0].ID != "item-1" || eventItems[0].PriceID != "price-1" || eventItems[0].Quantity != 2 {
		t.Fatalf("webhook cannot reconstruct order items: %+v", eventItems)
	}
}
