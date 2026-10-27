package consumer

import (
	"context"
	"errors"
	"testing"

	"github.com/jiahuipaung/gorder/payment/app"
	"github.com/jiahuipaung/gorder/payment/app/command"
	amqp "github.com/rabbitmq/amqp091-go"
)

type createPaymentHandlerStub struct {
	err   error
	calls int
	cmds  []command.CreatePayment
}

func (s *createPaymentHandlerStub) Handler(_ context.Context, cmd command.CreatePayment) (string, error) {
	s.calls++
	s.cmds = append(s.cmds, cmd)
	return "https://pay.example/session-1", s.err
}

type acknowledgementSpy struct {
	acks         int
	nacks        int
	rejects      int
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
	s.rejects++
	return nil
}

func TestHandleMessageAcknowledgesSuccessfulPaymentOnce(t *testing.T) {
	handler := &createPaymentHandlerStub{}
	retryCalls := 0
	consumer := Consumer{
		app: app.Application{Commands: app.Commands{CreatePayment: handler}},
		retryHandler: func(context.Context, *amqp.Channel, *amqp.Delivery) error {
			retryCalls++
			return nil
		},
	}
	acks := &acknowledgementSpy{}
	msg := amqp.Delivery{Acknowledger: acks, DeliveryTag: 1, Body: []byte(`{"id":"order-1"}`)}

	consumer.handleMessage(msg, amqp.Queue{Name: "order.created"}, nil)

	if handler.calls != 1 {
		t.Fatalf("expected handler once, got %d calls", handler.calls)
	}
	if retryCalls != 0 {
		t.Fatalf("expected no retry, got %d calls", retryCalls)
	}
	assertConfirmation(t, acks, 1, 0, nil)
}

func TestHandleMessageNacksMalformedJSONOnceWithoutRequeue(t *testing.T) {
	handler := &createPaymentHandlerStub{}
	retryCalls := 0
	consumer := Consumer{
		app: app.Application{Commands: app.Commands{CreatePayment: handler}},
		retryHandler: func(context.Context, *amqp.Channel, *amqp.Delivery) error {
			retryCalls++
			return nil
		},
	}
	acks := &acknowledgementSpy{}
	msg := amqp.Delivery{Acknowledger: acks, DeliveryTag: 2, Body: []byte(`{"id":`)}

	consumer.handleMessage(msg, amqp.Queue{Name: "order.created"}, nil)

	if handler.calls != 0 {
		t.Fatalf("expected handler not to run, got %d calls", handler.calls)
	}
	if retryCalls != 0 {
		t.Fatalf("expected retry not to run, got %d calls", retryCalls)
	}
	assertConfirmation(t, acks, 0, 1, []bool{false})
}

func TestHandleMessageAcknowledgesOriginalAfterSuccessfulRetryHandoff(t *testing.T) {
	handler := &createPaymentHandlerStub{err: errors.New("payment failed")}
	retryCalls := 0
	consumer := Consumer{
		app: app.Application{Commands: app.Commands{CreatePayment: handler}},
		retryHandler: func(context.Context, *amqp.Channel, *amqp.Delivery) error {
			retryCalls++
			return nil
		},
	}
	acks := &acknowledgementSpy{}
	msg := amqp.Delivery{Acknowledger: acks, DeliveryTag: 3, Body: []byte(`{"id":"order-1"}`)}

	consumer.handleMessage(msg, amqp.Queue{Name: "order.created"}, nil)

	if handler.calls != 1 {
		t.Fatalf("expected handler once, got %d calls", handler.calls)
	}
	if retryCalls != 1 {
		t.Fatalf("expected retry once, got %d calls", retryCalls)
	}
	assertConfirmation(t, acks, 1, 0, nil)
}

func TestHandleMessageRequeuesOriginalWhenRetryHandoffFails(t *testing.T) {
	handler := &createPaymentHandlerStub{err: errors.New("payment failed")}
	retryCalls := 0
	consumer := Consumer{
		app: app.Application{Commands: app.Commands{CreatePayment: handler}},
		retryHandler: func(context.Context, *amqp.Channel, *amqp.Delivery) error {
			retryCalls++
			return errors.New("retry publish failed")
		},
	}
	acks := &acknowledgementSpy{}
	msg := amqp.Delivery{Acknowledger: acks, DeliveryTag: 4, Body: []byte(`{"id":"order-1"}`)}

	consumer.handleMessage(msg, amqp.Queue{Name: "order.created"}, nil)

	if handler.calls != 1 {
		t.Fatalf("expected handler once, got %d calls", handler.calls)
	}
	if retryCalls != 1 {
		t.Fatalf("expected retry once, got %d calls", retryCalls)
	}
	assertConfirmation(t, acks, 0, 1, []bool{true})
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
