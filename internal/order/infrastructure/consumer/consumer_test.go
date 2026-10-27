package consumer

import (
	"context"
	"errors"
	"testing"

	"github.com/jiahuipaung/gorder/order/app"
	"github.com/jiahuipaung/gorder/order/app/command"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

type fakeMarkOrderPaidHandler struct {
	calls int
	err   error
}

func (h *fakeMarkOrderPaidHandler) Handler(_ context.Context, _ command.MarkOrderPaid) (bool, error) {
	h.calls++
	return true, h.err
}

type fakeAcknowledger struct {
	acks            int
	nacks           int
	lastNackRequeue bool
}

func (a *fakeAcknowledger) Ack(_ uint64, _ bool) error {
	a.acks++
	return nil
}

func (a *fakeAcknowledger) Nack(_ uint64, _ bool, requeue bool) error {
	a.nacks++
	a.lastNackRequeue = requeue
	return nil
}

func (a *fakeAcknowledger) Reject(_ uint64, _ bool) error {
	return nil
}

func TestConsumerAcknowledgesSuccessfulPaidEventOnce(t *testing.T) {
	t.Parallel()

	handler := &fakeMarkOrderPaidHandler{}
	consumer := NewConsumer(app.Application{Commands: app.Commands{
		MarkOrderPaid: handler,
	}})
	acknowledger := &fakeAcknowledger{}
	message := amqp.Delivery{
		Acknowledger: acknowledger,
		DeliveryTag:  1,
		Body: []byte(`{
			"ID":"order-1",
			"CustomerID":"customer-1",
			"Status":"paid",
			"PaymentLink":"payment-link-1"
		}`),
	}

	consumer.handleMessage(message, amqp.Queue{Name: "order.paid"}, nil)

	require.Equal(t, 1, handler.calls)
	require.Equal(t, 1, acknowledger.acks)
	require.Zero(t, acknowledger.nacks)
	require.Equal(t, 1, acknowledger.acks+acknowledger.nacks)
}

func TestConsumerNacksMalformedPayloadWithoutRequeue(t *testing.T) {
	t.Parallel()

	handler := &fakeMarkOrderPaidHandler{}
	consumer := NewConsumer(app.Application{Commands: app.Commands{
		MarkOrderPaid: handler,
	}})
	acknowledger := &fakeAcknowledger{}
	message := amqp.Delivery{
		Acknowledger: acknowledger,
		DeliveryTag:  1,
		Body:         []byte(`{"ID":`),
	}

	consumer.handleMessage(message, amqp.Queue{Name: "order.paid"}, nil)

	require.Zero(t, handler.calls)
	require.Zero(t, acknowledger.acks)
	require.Equal(t, 1, acknowledger.nacks)
	require.False(t, acknowledger.lastNackRequeue)
	require.Equal(t, 1, acknowledger.acks+acknowledger.nacks)
}

func TestConsumerNacksNonPaidPayloadWithoutRetry(t *testing.T) {
	t.Parallel()

	handler := &fakeMarkOrderPaidHandler{}
	retryCalls := 0
	consumer := &Consumer{
		app: app.Application{Commands: app.Commands{
			MarkOrderPaid: handler,
		}},
		retry: func(_ context.Context, _ *amqp.Channel, _ *amqp.Delivery) error {
			retryCalls++
			return nil
		},
	}
	acknowledger := &fakeAcknowledger{}
	message := amqp.Delivery{
		Acknowledger: acknowledger,
		DeliveryTag:  1,
		Body: []byte(`{
			"ID":"order-1",
			"CustomerID":"customer-1",
			"Status":"waiting_for_payment"
		}`),
	}

	consumer.handleMessage(message, amqp.Queue{Name: "order.paid"}, nil)

	require.Zero(t, handler.calls)
	require.Zero(t, retryCalls)
	require.Zero(t, acknowledger.acks)
	require.Equal(t, 1, acknowledger.nacks)
	require.False(t, acknowledger.lastNackRequeue)
	require.Equal(t, 1, acknowledger.acks+acknowledger.nacks)
}

func TestConsumerAcknowledgesOriginalAfterSuccessfulRetryHandoff(t *testing.T) {
	t.Parallel()

	handler := &fakeMarkOrderPaidHandler{err: errors.New("repository unavailable")}
	retryCalls := 0
	consumer := &Consumer{
		app: app.Application{Commands: app.Commands{
			MarkOrderPaid: handler,
		}},
		retry: func(_ context.Context, _ *amqp.Channel, _ *amqp.Delivery) error {
			retryCalls++
			return nil
		},
	}
	acknowledger := &fakeAcknowledger{}
	message := amqp.Delivery{
		Acknowledger: acknowledger,
		DeliveryTag:  1,
		Body: []byte(`{
			"ID":"order-1",
			"CustomerID":"customer-1",
			"Status":"paid",
			"PaymentLink":"payment-link-1"
		}`),
	}

	consumer.handleMessage(message, amqp.Queue{Name: "order.paid"}, nil)

	require.Equal(t, 1, handler.calls)
	require.Equal(t, 1, retryCalls)
	require.Equal(t, 1, acknowledger.acks)
	require.Zero(t, acknowledger.nacks)
	require.Equal(t, 1, acknowledger.acks+acknowledger.nacks)
}

func TestConsumerRequeuesOriginalWhenRetryHandoffFails(t *testing.T) {
	t.Parallel()

	handler := &fakeMarkOrderPaidHandler{err: errors.New("repository unavailable")}
	retryCalls := 0
	consumer := &Consumer{
		app: app.Application{Commands: app.Commands{
			MarkOrderPaid: handler,
		}},
		retry: func(_ context.Context, _ *amqp.Channel, _ *amqp.Delivery) error {
			retryCalls++
			return errors.New("retry publish failed")
		},
	}
	acknowledger := &fakeAcknowledger{}
	message := amqp.Delivery{
		Acknowledger: acknowledger,
		DeliveryTag:  1,
		Body: []byte(`{
			"ID":"order-1",
			"CustomerID":"customer-1",
			"Status":"paid",
			"PaymentLink":"payment-link-1"
		}`),
	}

	consumer.handleMessage(message, amqp.Queue{Name: "order.paid"}, nil)

	require.Equal(t, 1, handler.calls)
	require.Equal(t, 1, retryCalls)
	require.Zero(t, acknowledger.acks)
	require.Equal(t, 1, acknowledger.nacks)
	require.True(t, acknowledger.lastNackRequeue)
	require.Equal(t, 1, acknowledger.acks+acknowledger.nacks)
}
