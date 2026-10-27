package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jiahuipaung/gorder/common/broker"
	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
)

type Order struct {
	ID          string
	CustomerID  string
	Status      string
	PaymentLink string
	Items       []*orderpb.Item
}

type OrderService interface {
	MarkOrderReady(ctx context.Context, orderID, customerID string) error
	GetStatus(ctx context.Context, orderID, customerID string) (string, error)
}

type Consumer struct {
	orderGRPC    OrderService
	retryHandler RetryHandler
	cookHandler  CookHandler
	deferWait    func(context.Context)
}

type RetryHandler func(context.Context, *amqp.Channel, *amqp.Delivery) error

type CookHandler func(*Order)

func NewConsumer(orderGRPC OrderService) *Consumer {
	return &Consumer{
		orderGRPC: orderGRPC,
		retryHandler: func(ctx context.Context, ch *amqp.Channel, d *amqp.Delivery) error {
			return broker.HandleRetryToQueue(ctx, ch, d, broker.KitchenPaidQueue)
		},
		cookHandler: cook,
		deferWait: func(ctx context.Context) {
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			select {
			case <-ctx.Done():
			case <-timer.C:
			}
		},
	}
}

func (c *Consumer) Listen(ch *amqp.Channel) {
	q, err := ch.QueueDeclare(broker.KitchenPaidQueue, true, false, false, false, nil)
	if err != nil {
		logrus.Fatal(err)
	}

	if err = ch.QueueBind(q.Name, "", broker.EventOrderPaid, false, nil); err != nil {
		logrus.Fatal(err)
	}

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		logrus.Fatalf("fail to consume: queue=%s, err=%v", q.Name, err)
	}

	for msg := range msgs {
		c.handleMessage(msg, q, ch)
	}
	logrus.Fatal("kitchen consumer subscription closed")
}

func (c *Consumer) handleMessage(msg amqp.Delivery, q amqp.Queue, ch *amqp.Channel) {
	logrus.Infof("Kitchen receive a message from %s, message=%s", q.Name, string(msg.Body))
	ctx := broker.ExtractRabbitMQHeader(context.Background(), msg.Headers)
	t := otel.Tracer("rabbitMQ")
	mqCtx, span := t.Start(ctx, fmt.Sprintf("rabbitMQ.%s.consumer", q.Name))
	defer span.End()

	o := &Order{}
	if err := json.Unmarshal(msg.Body, o); err != nil {
		logrus.Warnf("fail to unmarshal msg to order, err=%v", err)
		_ = msg.Nack(false, false)
		return
	}
	if o.Status != "paid" {
		logrus.Warnf("order not paid, cannot cook: order=%s status=%s", o.ID, o.Status)
		_ = msg.Nack(false, false)
		return
	}
	persistedStatus, err := c.orderGRPC.GetStatus(mqCtx, o.ID, o.CustomerID)
	if err == nil && persistedStatus == "ready" {
		_ = msg.Ack(false)
		return
	}
	if err != nil || persistedStatus != "paid" {
		if err != nil {
			logrus.Warnf("kitchen: cannot verify paid order %s: %v", o.ID, err)
		}
		// This is not a failed fulfilment attempt. Order and Kitchen receive
		// separate paid deliveries, so Order may simply be behind. Keep the
		// original delivery until paid is visible; finite retry/DLQ would
		// strand a valid payment during a longer Order outage.
		if c.deferWait != nil {
			c.deferWait(mqCtx)
		}
		_ = msg.Nack(false, true)
		return
	}

	c.cookHandler(o)
	span.AddEvent(fmt.Sprintf("order_cook: %+v", o))
	if err := c.orderGRPC.MarkOrderReady(mqCtx, o.ID, o.CustomerID); err != nil {
		if err = c.retryHandler(mqCtx, ch, &msg); err != nil {
			logrus.Warnf("kitchen: fail to updateOrder and handle retry, err=%v", err)
			_ = msg.Nack(false, true)
			return
		}
		_ = msg.Ack(false)
		return
	}

	span.AddEvent("kitchen: update order")
	_ = msg.Ack(false)
	logrus.Infof("consume successfully")
}

func cook(o *Order) {
	logrus.Infof("cook start order=%s", o.ID)
	time.Sleep(5 * time.Second)
	logrus.Infof("cook done order=%s", o.ID)
}
