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
	app app.Application
}

func NewConsumer(app app.Application) *Consumer {
	return &Consumer{
		app: app,
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

func (c *Consumer) handleMessage(msg amqp.Delivery, q amqp.Queue, _ *amqp.Channel) {
	ctx := broker.ExtractRabbitMQHeader(context.Background(), msg.Headers)
	t := otel.Tracer("rabbitMQ")
	_, span := t.Start(ctx, fmt.Sprintf("rabbirMQ.%s.consume", q.Name))
	defer span.End()
	o := &domain.Order{}
	if err := json.Unmarshal(msg.Body, o); err != nil {
		logrus.Infof("Unmarshal msg.body into domain.order, err:%v", err)
		return
	}
	if _, err := c.app.Commands.UpdateOrder.Handler(ctx, command.UpdateOrder{
		Order: o,
		UpdateFn: func(ctx context.Context, order *domain.Order) (*domain.Order, error) {
			if err := order.IsPaid(); err != nil {
				return nil, err
			}
			return order, nil
		},
	}); err != nil {
		//	TODO: retry
		logrus.Infof("fail to update Order err:%v", err)
		_ = msg.Nack(false, false)
	}
	span.AddEvent("order updated")
	_ = msg.Ack(false)
	logrus.Infof("consume order:%+v", o)
}
