package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/jiahuipaung/gorder/order/entity"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type pendingStub struct{ order *domain.Order }

func (s pendingStub) MarkCreatePublished(context.Context, string, string) error { return nil }
func (s pendingStub) RejectStock(context.Context, string, string) error         { return nil }
func (s pendingStub) ListPending(_ context.Context, after string, _ int) ([]*domain.Order, error) {
	if after != "" {
		return nil, nil
	}
	return []*domain.Order{s.order}, nil
}

type rejectedPending struct {
	pendingStub
	rejected int
}

func (s *rejectedPending) RejectStock(context.Context, string, string) error {
	s.rejected++
	return nil
}

type exhaustedStock struct{ recoveryStock }

func (s *exhaustedStock) Reserve(context.Context, string, string, []*orderpb.ItemWithQuantity, int64) (*stockpb.ReservationResponse, error) {
	return nil, status.Error(codes.ResourceExhausted, "sold out")
}
func TestRecoverPendingRejectsDefiniteStockShortage(t *testing.T) {
	order := &domain.Order{ID: "order-2", CustomerID: "customer", Status: domain.StatusPending, Items: []*entity.Item{{ID: "sku", Quantity: 1}}, ExpiresAt: time.Now().Add(time.Hour)}
	repo := &rejectedPending{pendingStub: pendingStub{order: order}}
	publisher := &recoveryPublisher{}
	if err := RecoverPendingOnce(context.Background(), repo, &exhaustedStock{}, publisher); err != nil {
		t.Fatal(err)
	}
	if repo.rejected != 1 || publisher.count != 0 {
		t.Fatalf("rejected=%d published=%d", repo.rejected, publisher.count)
	}
}

type recoveryStock struct {
	attempts  int
	failFirst bool
}

func (s *recoveryStock) CheckIfItemsInStock(context.Context, []*orderpb.ItemWithQuantity) (*stockpb.CheckIfItemsInStockResponse, error) {
	return nil, nil
}
func (s *recoveryStock) GetItem(context.Context, []string) ([]*orderpb.Item, error) { return nil, nil }
func (s *recoveryStock) GetReservation(context.Context, string, string) (*stockpb.ReservationResponse, error) {
	return nil, nil
}
func (s *recoveryStock) Reserve(_ context.Context, id, owner string, _ []*orderpb.ItemWithQuantity, expiry int64) (*stockpb.ReservationResponse, error) {
	s.attempts++
	if s.failFirst && s.attempts == 1 {
		return nil, errors.New("stock temporarily unavailable")
	}
	return &stockpb.ReservationResponse{ReservationId: id, CustomerId: owner, Status: "RESERVED", ExpiresAtUnixMillis: expiry}, nil
}

type recoveryPublisher struct{ count int }

func (p *recoveryPublisher) PublishWithContext(context.Context, string, string, bool, bool, amqp.Publishing) error {
	p.count++
	return nil
}

func TestRecoverPendingRetriesPersistedIntent(t *testing.T) {
	order := &domain.Order{ID: "order-1", CustomerID: "customer", Status: domain.StatusPending, Items: []*entity.Item{{ID: "sku", Quantity: 1, PriceID: "price"}}, ExpiresAt: time.Now().Add(time.Hour)}
	repo := pendingStub{order: order}
	stock := &recoveryStock{failFirst: true}
	publisher := &recoveryPublisher{}
	if err := RecoverPendingOnce(context.Background(), repo, stock, publisher); err == nil {
		t.Fatal("transient stock error hidden")
	}
	if publisher.count != 0 {
		t.Fatal("published before reserve")
	}
	if err := RecoverPendingOnce(context.Background(), repo, stock, publisher); err != nil {
		t.Fatal(err)
	}
	if stock.attempts != 2 || publisher.count != 1 {
		t.Fatalf("attempts=%d published=%d", stock.attempts, publisher.count)
	}
}
