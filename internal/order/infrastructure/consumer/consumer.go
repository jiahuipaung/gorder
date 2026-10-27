package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/otel"

	"github.com/jiahuipaung/gorder/common/broker"
	"github.com/jiahuipaung/gorder/order/app"
	"github.com/jiahuipaung/gorder/order/app/command"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

type Consumer struct {
	app   app.Application
	retry RetryHandler
}

type RetryHandler func(context.Context, *amqp.Channel, *amqp.Delivery) error

func NewConsumer(app app.Application) *Consumer {
	return &Consumer{
		app:   app,
		retry: broker.HandleRetry,
	}
}

func (c *Consumer) Listen(ch *amqp.Channel) {
	q, err := ch.QueueDeclare(broker.EventOrderPaid, true, false, true, false, nil)
	if err != nil {
		logrus.Fatal(err)
	}
	err = ch.QueueBind(q.Name, "", broker.EventOrderPaid, true, nil)
	if err != nil {
		logrus.Fatal(err)
	}
	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		logrus.Fatal(err)
	}
	for msg := range msgs {
		c.handleMessage(msg, q, ch)
	}
}

func (c *Consumer) handleMessage(msg amqp.Delivery, q amqp.Queue, ch *amqp.Channel) {
	ctx := broker.ExtractRabbitMQHeader(context.Background(), msg.Headers)
	t := otel.Tracer("rabbitMQ")
	_, span := t.Start(ctx, fmt.Sprintf("rabbirMQ.%s.consume", q.Name))
	defer span.End()
	o := &domain.Order{}
	if err := json.Unmarshal(msg.Body, o); err != nil {
		logrus.Infof("Unmarshal msg.body into domain.order, err:%v", err)
		_ = msg.Nack(false, false)
		return
	}
	if o.Status != domain.StatusPaid {
		logrus.Infof("reject non-paid order event: order=%s status=%s", o.ID, o.Status)
		_ = msg.Nack(false, false)
		return
	}
	if _, err := c.app.Commands.MarkOrderPaid.Handler(ctx, command.MarkOrderPaid{
		Order: o,
	}); err != nil {
		logrus.Infof("fail to update Order err:%v", err)
		if err = c.retry(ctx, ch, &msg); err != nil {
			logrus.Warnf("error handle retry, messageID:%s, err=%v", msg.MessageId, err)
			_ = msg.Nack(false, true)
			return
		}
		_ = msg.Ack(false)
		return
	} else {
		span.AddEvent("order updated")
		_ = msg.Ack(false)
		logrus.Infof("consume order:%+v", o)
		return
	}
}
