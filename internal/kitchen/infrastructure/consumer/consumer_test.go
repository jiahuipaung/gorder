package consumer

import (
	"context"
	"errors"
	"testing"

	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	amqp "github.com/rabbitmq/amqp091-go"
)

type orderServiceStub struct {
	err    error
	calls  int
	orders []*orderpb.Order
}

func (s *orderServiceStub) UpdateOrder(_ context.Context, order *orderpb.Order) error {
	s.calls++
	s.orders = append(s.orders, order)
	return s.err
}

type acknowledgementSpy struct {
	acks         int
	nacks        int
	nackRequeues []bool
}

func (s *acknowledgementSpy) Ack(_ uint64, _ bool) error {
	s.acks++
	return nil
}

func (s *acknowledgementSpy) Nack(_ uint64, _ bool, requeue bool) error {
	s.nacks++
	s.nackRequeues = append(s.nackRequeues, requeue)
	return nil
}

func (s *acknowledgementSpy) Reject(_ uint64, _ bool) error {
	return nil
}

func TestHandleMessageNacksMalformedJSONWithoutSideEffects(t *testing.T) {
	orders := &orderServiceStub{}
	cookCalls := 0
	retryCalls := 0
	consumer := Consumer{
		orderGRPC: orders,
		cookHandler: func(*Order) {
			cookCalls++
		},
		retryHandler: func(context.Context, *amqp.Channel, *amqp.Delivery) error {
			retryCalls++
			return nil
		},
	}
	acks := &acknowledgementSpy{}
	msg := amqp.Delivery{Acknowledger: acks, DeliveryTag: 1, Body: []byte(`{"id":`)}

	consumer.handleMessage(msg, amqp.Queue{Name: "order.paid"}, nil)

	assertNoKitchenSideEffects(t, cookCalls, orders.calls, retryCalls)
	assertConfirmation(t, acks, 0, 1, []bool{false})
}

func TestHandleMessageNacksNonPaidOrderWithoutSideEffects(t *testing.T) {
	orders := &orderServiceStub{}
	cookCalls := 0
	retryCalls := 0
	consumer := Consumer{
		orderGRPC: orders,
		cookHandler: func(*Order) {
			cookCalls++
		},
		retryHandler: func(context.Context, *amqp.Channel, *amqp.Delivery) error {
			retryCalls++
			return nil
		},
	}
	acks := &acknowledgementSpy{}
	msg := amqp.Delivery{Acknowledger: acks, DeliveryTag: 2, Body: []byte(`{"ID":"order-1","Status":"pending"}`)}

	consumer.handleMessage(msg, amqp.Queue{Name: "order.paid"}, nil)

	assertNoKitchenSideEffects(t, cookCalls, orders.calls, retryCalls)
	assertConfirmation(t, acks, 0, 1, []bool{false})
}

func TestHandleMessageCooksAndAcknowledgesPaidOrder(t *testing.T) {
	orders := &orderServiceStub{}
	cookCalls := 0
	retryCalls := 0
	consumer := Consumer{
		orderGRPC: orders,
		cookHandler: func(*Order) {
			cookCalls++
		},
		retryHandler: func(context.Context, *amqp.Channel, *amqp.Delivery) error {
			retryCalls++
			return nil
		},
	}
	acks := &acknowledgementSpy{}
	msg := amqp.Delivery{Acknowledger: acks, DeliveryTag: 3, Body: []byte(`{"ID":"order-1","CustomerID":"customer-1","Status":"paid","PaymentLink":"payment-1"}`)}

	consumer.handleMessage(msg, amqp.Queue{Name: "order.paid"}, nil)

	if cookCalls != 1 {
		t.Fatalf("expected cook once, got %d calls", cookCalls)
	}
	if orders.calls != 1 {
		t.Fatalf("expected order service once, got %d calls", orders.calls)
	}
	if retryCalls != 0 {
		t.Fatalf("expected retry not to run, got %d calls", retryCalls)
	}
	updated := orders.orders[0]
	if updated.ID != "order-1" || updated.CustomerID != "customer-1" || updated.Status != "ready" || updated.PaymentLink != "payment-1" {
		t.Fatalf("unexpected ready order: %+v", updated)
	}
	assertConfirmation(t, acks, 1, 0, nil)
}

func TestHandleMessageAcknowledgesOriginalAfterSuccessfulRetryHandoff(t *testing.T) {
	orders := &orderServiceStub{err: errors.New("order update failed")}
	cookCalls := 0
	retryCalls := 0
	consumer := Consumer{
		orderGRPC: orders,
		cookHandler: func(*Order) {
			cookCalls++
		},
		retryHandler: func(context.Context, *amqp.Channel, *amqp.Delivery) error {
			retryCalls++
			return nil
		},
	}
	acks := &acknowledgementSpy{}
	msg := amqp.Delivery{Acknowledger: acks, DeliveryTag: 4, Body: []byte(`{"ID":"order-1","Status":"paid"}`)}

	consumer.handleMessage(msg, amqp.Queue{Name: "order.paid"}, nil)

	if cookCalls != 1 || orders.calls != 1 || retryCalls != 1 {
		t.Fatalf("expected cook, update, and retry once; got cook=%d update=%d retry=%d", cookCalls, orders.calls, retryCalls)
	}
	assertConfirmation(t, acks, 1, 0, nil)
}

func TestHandleMessageRequeuesOriginalWhenRetryHandoffFails(t *testing.T) {
	orders := &orderServiceStub{err: errors.New("order update failed")}
	cookCalls := 0
	retryCalls := 0
	consumer := Consumer{
		orderGRPC: orders,
		cookHandler: func(*Order) {
			cookCalls++
		},
		retryHandler: func(context.Context, *amqp.Channel, *amqp.Delivery) error {
			retryCalls++
			return errors.New("retry publish failed")
		},
	}
	acks := &acknowledgementSpy{}
	msg := amqp.Delivery{Acknowledger: acks, DeliveryTag: 5, Body: []byte(`{"ID":"order-1","Status":"paid"}`)}

	consumer.handleMessage(msg, amqp.Queue{Name: "order.paid"}, nil)

	if cookCalls != 1 || orders.calls != 1 || retryCalls != 1 {
		t.Fatalf("expected cook, update, and retry once; got cook=%d update=%d retry=%d", cookCalls, orders.calls, retryCalls)
	}
	assertConfirmation(t, acks, 0, 1, []bool{true})
}

func assertNoKitchenSideEffects(t *testing.T, cookCalls, updateCalls, retryCalls int) {
	t.Helper()
	if cookCalls != 0 || updateCalls != 0 || retryCalls != 0 {
		t.Fatalf("expected no side effects, got cook=%d update=%d retry=%d", cookCalls, updateCalls, retryCalls)
	}
}

func assertConfirmation(t *testing.T, got *acknowledgementSpy, wantAcks, wantNacks int, wantRequeues []bool) {
	t.Helper()
	if got.acks != wantAcks || got.nacks != wantNacks {
		t.Fatalf("expected %d ACK and %d NACK, got %d ACK and %d NACK", wantAcks, wantNacks, got.acks, got.nacks)
	}
	if got.acks+got.nacks != 1 {
		t.Fatalf("expected exactly one terminal confirmation, got %d", got.acks+got.nacks)
	}
	if len(got.nackRequeues) != len(wantRequeues) {
		t.Fatalf("expected requeue flags %v, got %v", wantRequeues, got.nackRequeues)
	}
	for i := range wantRequeues {
		if got.nackRequeues[i] != wantRequeues[i] {
			t.Fatalf("expected requeue flags %v, got %v", wantRequeues, got.nackRequeues)
		}
	}
}
