package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jiahuipaung/gorder/common/broker"
	"github.com/jiahuipaung/gorder/common/decorator"
	"github.com/jiahuipaung/gorder/order/app/query"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/jiahuipaung/gorder/order/entity"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"order/convertor"
)

type CreateOrder struct {
	CustomerID string
	Items      []*entity.ItemWithQuantity
}

type CreateOrderResult struct {
	OrderID string
}

type CreateOrderHandler decorator.CommandHandler[CreateOrder, *CreateOrderResult]

type createOrderHandler struct {
	orderRepo domain.Repository
	stockGrpc query.StockService
	channel   *amqp.Channel
}

func NewCreateOrderHandler(
	orderRepo domain.Repository,
	stockGrpc query.StockService,
	logger *logrus.Entry,
	channel *amqp.Channel,
	metricsClient decorator.MetricsClient,
) CreateOrderHandler {
	if orderRepo == nil {
		panic("orderRepo is nil")
	}
	if stockGrpc == nil {
		panic("stockGrpc is nil")
	}
	if channel == nil {
		panic("channel is nil")
	}
	return decorator.ApplyCommandDecorators[CreateOrder, *CreateOrderResult](
		createOrderHandler{
			orderRepo: orderRepo,
			stockGrpc: stockGrpc,
			channel:   channel,
		},
		logger,
		metricsClient,
	)
}

func (c createOrderHandler) Handler(ctx context.Context, cmd CreateOrder) (*CreateOrderResult, error) {
	q, err := c.channel.QueueDeclare(broker.EventOrderCreate, true, false, false, false, nil)
	if err != nil {
		return nil, err
	}

	t := otel.Tracer("rabbitMQ")
	ctx, span := t.Start(ctx, fmt.Sprintf("rabbitMQ.%s.publish", q.Name))
	defer span.End()

	validItems, err := c.validate(ctx, cmd.Items)
	if err != nil {
		return nil, err
	}
	o, err := c.orderRepo.Create(ctx, &domain.Order{
		CustomerID: cmd.CustomerID,
		Items:      validItems,
	})
	if err != nil {
		return nil, err
	}

	marshalledOrder, err := json.Marshal(o)
	if err != nil {
		return nil, err
	}

	header := broker.InjectRabbitMQHeader(ctx)
	err = c.channel.PublishWithContext(ctx, "", q.Name, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         marshalledOrder,
		Headers:      header,
	})
	if err != nil {
		return nil, err
	}
	return &CreateOrderResult{OrderID: o.ID}, nil
}

func (c createOrderHandler) validate(ctx context.Context, items []*entity.ItemWithQuantity) ([]*entity.Item, error) {
	if len(items) < 1 {
		return nil, errors.New("invalid item count")
	}
	items = packItems(items)
	resp, err := c.stockGrpc.CheckIfItemsInStock(ctx, convertor.NewItemWithQuantityConverter().EntitiesToProto(items))
	if err != nil {
		return nil, err
	}
	return convertor.NewItemConvertor().ProtoToEntities(resp.Items), nil
}

func packItems(items []*entity.ItemWithQuantity) []*entity.ItemWithQuantity {
	merged := make(map[string]int32)
	for _, item := range items {
		merged[item.ID] += item.Quantity
	}
	var result []*entity.ItemWithQuantity
	for id, quantity := range merged {
		result = append(result, &entity.ItemWithQuantity{
			ID:       id,
			Quantity: quantity,
		})
	}
	return result
}
