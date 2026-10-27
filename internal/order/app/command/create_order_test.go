package command

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/jiahuipaung/gorder/common/broker"
	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
	"github.com/jiahuipaung/gorder/order/adapters"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/jiahuipaung/gorder/order/entity"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNewOrderForCreationStartsPending(t *testing.T) {
	t.Parallel()

	items := []*entity.Item{{ID: "item-1", Quantity: 2}}
	order := newOrderForCreation("customer-1", items)

	require.Equal(t, "customer-1", order.CustomerID)
	require.Equal(t, domain.StatusPending, order.Status)
	require.Same(t, items[0], order.Items[0])
}

func TestCreateOrderRequiresCustomerBeforeBrokerAccess(t *testing.T) {
	_, err := (createOrderHandler{}).Handler(context.Background(), CreateOrder{CustomerID: "  "})
	if err == nil {
		t.Fatal("blank customer ID accepted")
	}
}

type validationStock struct {
	response *stockpb.CheckIfItemsInStockResponse
}

func (s validationStock) CheckIfItemsInStock(context.Context, []*orderpb.ItemWithQuantity) (*stockpb.CheckIfItemsInStockResponse, error) {
	return s.response, nil
}
func (s validationStock) GetItem(context.Context, []string) ([]*orderpb.Item, error) { return nil, nil }
func (s validationStock) GetReservation(context.Context, string, string) (*stockpb.ReservationResponse, error) {
	return nil, nil
}
func (s validationStock) Reserve(context.Context, string, string, []*orderpb.ItemWithQuantity, int64) (*stockpb.ReservationResponse, error) {
	return nil, nil
}

type checkoutStock struct {
	validationStock
	reservedID string
	reserveErr error
}

func (s *checkoutStock) Reserve(_ context.Context, id, customer string, _ []*orderpb.ItemWithQuantity, expires int64) (*stockpb.ReservationResponse, error) {
	s.reservedID = id
	if s.reserveErr != nil {
		return nil, s.reserveErr
	}
	return &stockpb.ReservationResponse{ReservationId: id, CustomerId: customer, Status: "RESERVED", ExpiresAtUnixMillis: expires}, nil
}

type recordedPublisher struct{ count int }

func (p *recordedPublisher) PublishWithContext(_ context.Context, exchange, key string, mandatory, immediate bool, _ amqp.Publishing) error {
	if exchange != "" || key != broker.EventOrderCreate || !mandatory || immediate {
		return errors.New("unexpected routing")
	}
	p.count++
	return nil
}
func TestCreateReservesBeforePublishing(t *testing.T) {
	stock := &checkoutStock{validationStock: validationStock{response: &stockpb.CheckIfItemsInStockResponse{Items: []*orderpb.Item{{ID: "sku", Quantity: 2, PriceID: "price"}}}}}
	publisher := &recordedPublisher{}
	repo := adapters.NewMemoryOrderRepository()
	h := createOrderHandler{orderRepo: repo, stockGrpc: stock, publisher: publisher}
	r, err := h.Handler(context.Background(), CreateOrder{CustomerID: "customer", Items: []*entity.ItemWithQuantity{{ID: "sku", Quantity: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	if r.OrderID == "" || stock.reservedID != r.OrderID || publisher.count != 1 {
		t.Fatalf("reservation/publish mismatch: result=%v reserved=%q publishes=%d", r, stock.reservedID, publisher.count)
	}
	o, err := repo.Get(context.Background(), r.OrderID, "customer")
	if err != nil {
		t.Fatal(err)
	}
	if o.ExpiresAt.Before(time.Now()) {
		t.Fatal("expiry not persisted with order")
	}
}
func TestCreateDoesNotPublishAfterReservationFailure(t *testing.T) {
	stock := &checkoutStock{validationStock: validationStock{response: &stockpb.CheckIfItemsInStockResponse{Items: []*orderpb.Item{{ID: "sku", Quantity: 1, PriceID: "price"}}}}, reserveErr: errors.New("no stock")}
	publisher := &recordedPublisher{}
	h := createOrderHandler{orderRepo: adapters.NewMemoryOrderRepository(), stockGrpc: stock, publisher: publisher}
	if result, err := h.Handler(context.Background(), CreateOrder{CustomerID: "customer", Items: []*entity.ItemWithQuantity{{ID: "sku", Quantity: 1}}}); err != nil || result.OrderID == "" {
		t.Fatalf("pending order ID lost: result=%v err=%v", result, err)
	}
	if publisher.count != 0 {
		t.Fatal("payment event published without reservation")
	}
}

func TestCreateMarksDefiniteShortageRejected(t *testing.T) {
	stock := &checkoutStock{validationStock: validationStock{response: &stockpb.CheckIfItemsInStockResponse{Items: []*orderpb.Item{{ID: "sku", Quantity: 1, PriceID: "price"}}}}, reserveErr: status.Error(codes.ResourceExhausted, "sold out")}
	repo := adapters.NewMemoryOrderRepository()
	publisher := &recordedPublisher{}
	h := createOrderHandler{orderRepo: repo, stockGrpc: stock, publisher: publisher}
	if _, err := h.Handler(context.Background(), CreateOrder{CustomerID: "customer", Items: []*entity.ItemWithQuantity{{ID: "sku", Quantity: 1}}}); err == nil {
		t.Fatal("sold-out checkout accepted")
	}
	o, err := repo.Get(context.Background(), stock.reservedID, "customer")
	if err != nil || o.Status != domain.StatusStockRejected || publisher.count != 0 {
		t.Fatalf("order=%v error=%v publishes=%d", o, err, publisher.count)
	}
}

func TestValidateRejectsInvalidCartBeforeStockCall(t *testing.T) {
	for name, items := range map[string][]*entity.ItemWithQuantity{
		"nil item":  {nil},
		"empty SKU": {{Quantity: 1}},
		"zero":      {{ID: "a", Quantity: 0}},
		"negative":  {{ID: "a", Quantity: -1}},
		"overflow":  {{ID: "a", Quantity: math.MaxInt32}, {ID: "a", Quantity: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			h := createOrderHandler{stockGrpc: validationStock{response: &stockpb.CheckIfItemsInStockResponse{Items: []*orderpb.Item{{ID: "a", Quantity: 1, PriceID: "price_a"}}}}}
			_, err := h.validate(context.Background(), items)
			if err == nil {
				t.Fatal("invalid cart accepted")
			}
		})
	}
}

func TestValidateRejectsIncompleteStockResponse(t *testing.T) {
	for name, response := range map[string]*stockpb.CheckIfItemsInStockResponse{
		"nil response":   nil,
		"missing":        {Items: nil},
		"nil item":       {Items: []*orderpb.Item{nil}},
		"wrong SKU":      {Items: []*orderpb.Item{{ID: "b", Quantity: 2, PriceID: "price_b"}}},
		"wrong quantity": {Items: []*orderpb.Item{{ID: "a", Quantity: 1, PriceID: "price_a"}}},
		"no price":       {Items: []*orderpb.Item{{ID: "a", Quantity: 2}}},
		"duplicate":      {Items: []*orderpb.Item{{ID: "a", Quantity: 2, PriceID: "price_a"}, {ID: "a", Quantity: 2, PriceID: "price_a"}}},
	} {
		t.Run(name, func(t *testing.T) {
			h := createOrderHandler{stockGrpc: validationStock{response: response}}
			_, err := h.validate(context.Background(), []*entity.ItemWithQuantity{{ID: "a", Quantity: 2}})
			if err == nil {
				t.Fatal("inconsistent stock result accepted")
			}
		})
	}
}

func TestValidateCombinesDuplicateSKUs(t *testing.T) {
	h := createOrderHandler{stockGrpc: validationStock{response: &stockpb.CheckIfItemsInStockResponse{Items: []*orderpb.Item{{ID: "a", Quantity: 3, PriceID: "price_a"}}}}}
	got, err := h.validate(context.Background(), []*entity.ItemWithQuantity{{ID: "a", Quantity: 1}, {ID: "a", Quantity: 2}})
	if err != nil || len(got) != 1 || got[0].Quantity != 3 {
		t.Fatalf("got %v, %v", got, err)
	}
}
