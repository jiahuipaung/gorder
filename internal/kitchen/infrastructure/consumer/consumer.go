package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jiahuipaung/gorder/common/broker"
	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"time"
)

type Order struct {
	ID          string
	CustomerID  string
	Status      string
	PaymentLink string
	Items       []*orderpb.Item
}

type OrderService interface {
	UpdateOrder(ctx context.Context, req *orderpb.Order) error
}

type Consumer struct {
	orderGRPC OrderService
}

func NewConsumer(orderGRPC OrderService) *Consumer {
	return &Consumer{
		orderGRPC: orderGRPC,
	}
}

func (c *Consumer) Listen(ch *amqp.Channel) {
	q, err := ch.QueueDeclare("", true, false, true, false, nil)
	if err != nil {
		logrus.Fatal(err)
	}

	if err = ch.QueueBind(q.Name, "", broker.EventOrderPaid, false, nil); err != nil {
		logrus.Fatal(err)
	}

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		logrus.Warnf("fail to consume: queue=%s, err=%v", q.Name, err)
	}

	for msg := range msgs {
		c.handleMessage(msg, q, ch)
	}
}

func (c *Consumer) handleMessage(msg amqp.Delivery, q amqp.Queue, ch *amqp.Channel) {
	var err error
	logrus.Infof("Kitchen receive a message from %s, message=%s", q.Name, string(msg.Body))
	ctx := broker.ExtractRabbitMQHeader(context.Background(), msg.Headers)
	t := otel.Tracer("rabbitMQ")
	mqCtx, span := t.Start(ctx, fmt.Sprintf("rabbitMQ.%s.consumer", q.Name))

	defer func() {
		span.End()
		if err != nil {
			_ = msg.Nack(false, false)
		} else {
			_ = msg.Ack(false)
		}
	}()

	o := &Order{}
	if err = json.Unmarshal(msg.Body, o); err != nil {
		logrus.Warnf("fail to unmarshal msg to order, err=%v", err)
		_ = msg.Nack(false, false)
	}
	if o.Status != "paid" {
		err = errors.New("order not paid, cannot cook ")
		return
	}

	cook(o)
	span.AddEvent(fmt.Sprintf("order_cook: %+v", o))
	if err = c.orderGRPC.UpdateOrder(mqCtx, &orderpb.Order{
		ID:          o.ID,
		CustomerID:  o.CustomerID,
		Status:      "ready",
		PaymentLink: o.PaymentLink,
		Items:       o.Items,
	}); err != nil {
		if err = broker.HandleRetry(mqCtx, ch, &msg); err != nil {
			logrus.Warnf("kitchen: fail to updateOrder and handle retry, err=%v", err)
		}
		return
	}

	span.AddEvent("kitchen: update order")
	logrus.Infof("consume successfully")
}

func cook(o *Order) {
	logrus.Infof("cook start order=%s", o.ID)
	time.Sleep(5 * time.Second)
	logrus.Infof("cook done order=%s", o.ID)
}
