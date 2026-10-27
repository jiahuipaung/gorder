package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jiahuipaung/gorder/common/broker"
	"github.com/jiahuipaung/gorder/common/decorator"
	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/order/app/query"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/jiahuipaung/gorder/order/entity"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"math"
	"order/convertor"
	"sort"
	"strings"
	"time"
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
	publisher OrderPublisher
}

type OrderPublisher interface {
	PublishWithContext(context.Context, string, string, bool, bool, amqp.Publishing) error
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
	publisher, err := broker.ConfirmedPublisherFor(channel)
	if err != nil {
		panic(err)
	}
	return decorator.ApplyCommandDecorators[CreateOrder, *CreateOrderResult](
		createOrderHandler{
			orderRepo: orderRepo,
			stockGrpc: stockGrpc,
			publisher: publisher,
		},
		logger,
		metricsClient,
	)
}

func (c createOrderHandler) Handler(ctx context.Context, cmd CreateOrder) (*CreateOrderResult, error) {
	if strings.TrimSpace(cmd.CustomerID) == "" {
		return nil, errors.New("customer ID is required")
	}
	t := otel.Tracer("rabbitMQ")
	ctx, span := t.Start(ctx, fmt.Sprintf("rabbitMQ.%s.publish", broker.EventOrderCreate))
	defer span.End()

	validItems, err := c.validate(ctx, cmd.Items)
	if err != nil {
		return nil, err
	}
	o, err := c.orderRepo.Create(ctx, newOrderForCreation(cmd.CustomerID, validItems))
	if err != nil {
		return nil, err
	}

	if err := EnsurePending(ctx, o, c.stockGrpc, c.publisher); err != nil {
		if status.Code(err) == codes.ResourceExhausted {
			if rejecter, ok := c.orderRepo.(interface {
				RejectStock(context.Context, string, string) error
			}); ok {
				if rejectErr := rejecter.RejectStock(ctx, o.ID, o.CustomerID); rejectErr != nil {
					return nil, errors.Join(err, rejectErr)
				}
			}
			return nil, err
		}
		logrus.WithError(err).Warnf("order %s accepted pending recovery", o.ID)
		return &CreateOrderResult{OrderID: o.ID}, nil
	}
	if marker, ok := c.orderRepo.(interface {
		MarkCreatePublished(context.Context, string, string) error
	}); ok {
		if err := marker.MarkCreatePublished(ctx, o.ID, o.CustomerID); err != nil {
			logrus.WithError(err).Warnf("order %s publish marker pending recovery", o.ID)
		}
	}
	return &CreateOrderResult{OrderID: o.ID}, nil
}

// EnsurePending retries the same persisted order intent without deducting stock twice.
func EnsurePending(ctx context.Context, o *domain.Order, stock query.StockService, publisher OrderPublisher) error {
	if o == nil || o.ID == "" || o.CustomerID == "" || o.Status != domain.StatusPending || o.ExpiresAt.IsZero() {
		return errors.New("valid pending order intent is required")
	}
	if !o.ExpiresAt.After(time.Now()) {
		return errors.New("pending order expired; reconcile payment before release")
	}
	items := make([]*orderpb.ItemWithQuantity, 0, len(o.Items))
	for _, item := range o.Items {
		if item == nil || item.ID == "" || item.Quantity <= 0 {
			return errors.New("invalid persisted order item")
		}
		items = append(items, &orderpb.ItemWithQuantity{ID: item.ID, Quantity: item.Quantity})
	}
	reserved, err := stock.Reserve(ctx, o.ID, o.CustomerID, items, o.ExpiresAt.UnixMilli())
	if err != nil {
		return err
	}
	if reserved == nil || reserved.ReservationId != o.ID || reserved.CustomerId != o.CustomerID || (reserved.Status != "RESERVED" && reserved.Status != "CONFIRMED") {
		return errors.New("stock reservation response does not match order")
	}
	marshalledOrder, err := json.Marshal(o)
	if err != nil {
		return err
	}

	header := broker.InjectRabbitMQHeader(ctx)
	err = publisher.PublishWithContext(ctx, "", broker.EventOrderCreate, true, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         marshalledOrder,
		Headers:      header,
	})
	if err != nil {
		return err
	}
	return nil
}

func newOrderForCreation(customerID string, items []*entity.Item) *domain.Order {
	return &domain.Order{
		CustomerID: customerID,
		Status:     domain.StatusPending,
		Items:      items,
		ExpiresAt:  time.Now().UTC().Add(15 * time.Minute).Truncate(time.Millisecond),
	}
}

func (c createOrderHandler) validate(ctx context.Context, items []*entity.ItemWithQuantity) ([]*entity.Item, error) {
	if len(items) < 1 {
		return nil, errors.New("invalid item count")
	}
	items, err := packItems(items)
	if err != nil {
		return nil, err
	}
	resp, err := c.stockGrpc.CheckIfItemsInStock(ctx, convertor.NewItemWithQuantityConverter().EntitiesToProto(items))
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Items) != len(items) {
		return nil, errors.New("stock response does not match cart")
	}
	expected := make(map[string]int32, len(items))
	for _, item := range items {
		expected[item.ID] = item.Quantity
	}
	for _, item := range resp.Items {
		if item == nil || item.PriceID == "" || expected[item.ID] != item.Quantity || item.Quantity <= 0 {
			return nil, errors.New("invalid stock item or missing price")
		}
		delete(expected, item.ID)
	}
	if len(expected) != 0 {
		return nil, errors.New("incomplete stock response")
	}
	return convertor.NewItemConvertor().ProtoToEntities(resp.Items), nil
}

func packItems(items []*entity.ItemWithQuantity) ([]*entity.ItemWithQuantity, error) {
	merged := make(map[string]int32)
	for _, item := range items {
		if item == nil || strings.TrimSpace(item.ID) == "" || item.Quantity <= 0 {
			return nil, errors.New("SKU and positive quantity are required")
		}
		if int64(merged[item.ID])+int64(item.Quantity) > math.MaxInt32 {
			return nil, errors.New("item quantity exceeds supported limit")
		}
		merged[item.ID] += item.Quantity
	}
	var result []*entity.ItemWithQuantity
	for id, quantity := range merged {
		result = append(result, &entity.ItemWithQuantity{
			ID:       id,
			Quantity: quantity,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}
