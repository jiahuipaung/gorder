package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/otel"

	"github.com/jiahuipaung/gorder/common/broker"
	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/payment/app"
	"github.com/jiahuipaung/gorder/payment/app/command"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

type Consumer struct {
	app app.Application
}

func NewConsumer(app app.Application) *Consumer {
	return &Consumer{
		app: app,
	}
}

func (c *Consumer) Listen(ch *amqp.Channel) {
	q, err := ch.QueueDeclare(broker.EventOrderCreate, true, false, false, false, nil)
	if err != nil {
		logrus.Fatal(err)
	}
	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		logrus.Warnf("fail to consume: queue=%s, err=%v", q.Name, err)
	}

	for msg := range msgs {
		c.handleMessage(msg, q)
	}
}

func (c *Consumer) handleMessage(msg amqp.Delivery, q amqp.Queue) {
	logrus.Infof("Payment receive a message from %s, message=%s", q.Name, string(msg.Body))
	ctx := broker.ExtractRabbitMQHeader(context.Background(), msg.Headers)
	tr := otel.Tracer("rabbitMQ")
	_, span := tr.Start(ctx, fmt.Sprintf("rabbitMQ.%s.consumer", q.Name))
	defer span.End()
	o := &orderpb.Order{}
	if err := json.Unmarshal(msg.Body, o); err != nil {
		logrus.Warnf("fail to unmarshal msg to order, err=%v", err)
		_ = msg.Nack(false, false)
	}
	if _, err := c.app.Commands.CreatePayment.Handler(ctx, command.CreatePayment{Order: o}); err != nil {
		// TODO: retry
		logrus.Warnf("fail to create payment, err=%v", err)
		_ = msg.Nack(false, false)
	}
	span.AddEvent("payment created")
	_ = msg.Ack(false)
	logrus.Infof("consume successfully")
}
