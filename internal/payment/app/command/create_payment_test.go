package command

import (
	"context"
	"errors"
	"testing"

	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
)

type processorStub struct {
	link   string
	err    error
	calls  int
	orders []*orderpb.Order
}

func (s *processorStub) CreatePaymentLink(_ context.Context, order *orderpb.Order) (string, error) {
	s.calls++
	s.orders = append(s.orders, order)
	return s.link, s.err
}

type orderServiceStub struct {
	err         error
	calls       int
	orderIDs    []string
	customerIDs []string
	links       []string
}

func (s *orderServiceStub) AttachPaymentSession(
	_ context.Context,
	orderID, customerID, paymentLink string,
) error {
	s.calls++
	s.orderIDs = append(s.orderIDs, orderID)
	s.customerIDs = append(s.customerIDs, customerID)
	s.links = append(s.links, paymentLink)
	return s.err
}

func TestCreatePaymentRejectsNilOrderBeforeCallingDependencies(t *testing.T) {
	processor := &processorStub{err: errors.New("processor must not be called")}
	orders := &orderServiceStub{}
	handler := createPaymentHandler{processor: processor, orderGrpc: orders}

	_, err := handler.Handler(context.Background(), CreatePayment{})

	if err == nil {
		t.Fatal("expected nil order to return an error")
	}
	if processor.calls != 0 {
		t.Fatalf("expected processor not to be called, got %d calls", processor.calls)
	}
	if orders.calls != 0 {
		t.Fatalf("expected order service not to be called, got %d calls", orders.calls)
	}
}

func TestCreatePaymentReturnsProcessorErrorWithoutUpdatingOrder(t *testing.T) {
	processorErr := errors.New("stripe unavailable")
	processor := &processorStub{err: processorErr}
	orders := &orderServiceStub{}
	handler := createPaymentHandler{processor: processor, orderGrpc: orders}
	order := &orderpb.Order{ID: "order-1", CustomerID: "customer-1"}

	_, err := handler.Handler(context.Background(), CreatePayment{Order: order})

	if !errors.Is(err, processorErr) {
		t.Fatalf("expected processor error, got %v", err)
	}
	if processor.calls != 1 {
		t.Fatalf("expected processor once, got %d calls", processor.calls)
	}
	if orders.calls != 0 {
		t.Fatalf("expected order service not to be called, got %d calls", orders.calls)
	}
}

func TestCreatePaymentReturnsAttachPaymentSessionError(t *testing.T) {
	updateErr := errors.New("order service unavailable")
	processor := &processorStub{link: "https://pay.example/session-1"}
	orders := &orderServiceStub{err: updateErr}
	handler := createPaymentHandler{processor: processor, orderGrpc: orders}
	order := &orderpb.Order{ID: "order-1", CustomerID: "customer-1"}

	_, err := handler.Handler(context.Background(), CreatePayment{Order: order})

	if !errors.Is(err, updateErr) {
		t.Fatalf("expected order update error, got %v", err)
	}
	if orders.calls != 1 {
		t.Fatalf("expected order service once, got %d calls", orders.calls)
	}
}

func TestCreatePaymentAttachesSessionAndReturnsLink(t *testing.T) {
	link := "https://pay.example/session-1"
	items := []*orderpb.Item{{ID: "item-1", Quantity: 2}}
	processor := &processorStub{link: link}
	orders := &orderServiceStub{}
	handler := createPaymentHandler{processor: processor, orderGrpc: orders}
	order := &orderpb.Order{
		ID:         "order-1",
		CustomerID: "customer-1",
		Status:     "pending",
		Items:      items,
	}

	gotLink, err := handler.Handler(context.Background(), CreatePayment{Order: order})

	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if gotLink != link {
		t.Fatalf("expected link %q, got %q", link, gotLink)
	}
	if processor.calls != 1 {
		t.Fatalf("expected processor once, got %d calls", processor.calls)
	}
	if orders.calls != 1 {
		t.Fatalf("expected order service once, got %d calls", orders.calls)
	}
	if orders.orderIDs[0] != order.ID || orders.customerIDs[0] != order.CustomerID {
		t.Fatalf("unexpected order identity: order=%q customer=%q", orders.orderIDs[0], orders.customerIDs[0])
	}
	if orders.links[0] != link {
		t.Fatalf("expected payment link %q, got %q", link, orders.links[0])
	}
}
