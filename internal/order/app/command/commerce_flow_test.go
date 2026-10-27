package command

import (
	"context"
	"testing"

	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
	"github.com/jiahuipaung/gorder/order/adapters"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/jiahuipaung/gorder/order/entity"
	"github.com/stretchr/testify/require"
)

// This is an offline application-contract scenario. The real MySQL and
// RabbitMQ boundaries are exercised separately by their integration suites.
type flowStock struct {
	validationStock
	reservationID string
	owner         string
	state         string
}

func (s *flowStock) Reserve(_ context.Context, id, owner string, _ []*orderpb.ItemWithQuantity, expiry int64) (*stockpb.ReservationResponse, error) {
	s.reservationID, s.owner, s.state = id, owner, "RESERVED"
	return &stockpb.ReservationResponse{ReservationId: id, CustomerId: owner, Status: s.state, ExpiresAtUnixMillis: expiry}, nil
}

func (s *flowStock) GetReservation(_ context.Context, id, owner string) (*stockpb.ReservationResponse, error) {
	if id != s.reservationID || owner != s.owner {
		return nil, nil
	}
	return &stockpb.ReservationResponse{ReservationId: id, CustomerId: owner, Status: s.state}, nil
}

func TestOfflineCommerceFlowRequiresConfirmedStockBeforePaidAndReady(t *testing.T) {
	ctx := context.Background()
	repo := adapters.NewMemoryOrderRepository()
	stock := &flowStock{validationStock: validationStock{response: &stockpb.CheckIfItemsInStockResponse{
		Items: []*orderpb.Item{{ID: "sku-1", Quantity: 2, PriceID: "price-test"}},
	}}}
	publisher := &recordedPublisher{}
	created, err := (createOrderHandler{orderRepo: repo, stockGrpc: stock, publisher: publisher}).Handler(ctx,
		CreateOrder{CustomerID: "customer-1", Items: []*entity.ItemWithQuantity{{ID: "sku-1", Quantity: 2}}})
	require.NoError(t, err)
	require.Equal(t, created.OrderID, stock.reservationID)
	require.Equal(t, 1, publisher.count)

	changed, err := repo.AttachPaymentSession(ctx, created.OrderID, "customer-1", "https://pay.test/session")
	require.NoError(t, err)
	require.True(t, changed)
	paidEvent := MarkOrderPaid{Order: &domain.Order{ID: created.OrderID, CustomerID: "customer-1", Status: domain.StatusPaid}}
	paid := markOrderPaidHandler{orderRepo: repo, stock: stock}
	changed, err = paid.Handler(ctx, paidEvent)
	require.Error(t, err)
	require.False(t, changed)

	stock.state = "CONFIRMED" // signed, paid provider callback confirmed the reservation
	changed, err = paid.Handler(ctx, paidEvent)
	require.NoError(t, err)
	require.True(t, changed)
	changed, err = paid.Handler(ctx, paidEvent)
	require.NoError(t, err)
	require.False(t, changed)

	ready := markOrderReadyHandler{orderRepo: repo}
	changed, err = ready.Handler(ctx, MarkOrderReady{OrderID: created.OrderID, CustomerID: "customer-1"})
	require.NoError(t, err)
	require.True(t, changed)
	changed, err = ready.Handler(ctx, MarkOrderReady{OrderID: created.OrderID, CustomerID: "customer-1"})
	require.NoError(t, err)
	require.False(t, changed)
	stored, err := repo.Get(ctx, created.OrderID, "customer-1")
	require.NoError(t, err)
	require.Equal(t, domain.StatusReady, stored.Status)
	require.Equal(t, "https://pay.test/session", stored.PaymentLink)
}
