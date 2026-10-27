package ports

import (
	"context"
	"testing"
	"time"

	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
	"github.com/jiahuipaung/gorder/stock/app"
	"github.com/jiahuipaung/gorder/stock/reservation"
)

type reservationStub struct{ got reservation.Request }

func (s *reservationStub) Reserve(_ context.Context, req reservation.Request) (*reservation.Reservation, error) {
	s.got = req
	return &reservation.Reservation{ID: req.ID, CustomerID: req.CustomerID, Status: reservation.Reserved, ExpiresAt: req.ExpiresAt}, nil
}
func (s *reservationStub) Confirm(_ context.Context, id, owner string) (*reservation.Reservation, error) {
	return &reservation.Reservation{ID: id, CustomerID: owner, Status: reservation.Confirmed}, nil
}
func (s *reservationStub) Release(_ context.Context, id, owner string) (*reservation.Reservation, error) {
	return &reservation.Reservation{ID: id, CustomerID: owner, Status: reservation.Released}, nil
}
func (s *reservationStub) Get(_ context.Context, id, owner string) (*reservation.Reservation, error) {
	return &reservation.Reservation{ID: id, CustomerID: owner, Status: reservation.Reserved}, nil
}

func TestReservationRPCPreservesIdentityAndQuantity(t *testing.T) {
	store := &reservationStub{}
	server := NewGRPCServer(app.Application{Reservations: store})
	expires := time.Now().Add(time.Hour).UnixMilli()
	r, err := server.Reserve(context.Background(), &stockpb.ReserveRequest{
		ReservationId: "order-1", CustomerId: "customer-1", ExpiresAtUnixMillis: expires,
		Items: []*orderpb.ItemWithQuantity{{ID: "sku-1", Quantity: 2}},
	})
	if err != nil || r.Status != reservation.Reserved || store.got.Items[0].SKU != "sku-1" || store.got.Items[0].Quantity != 2 || store.got.ExpiresAt.UnixMilli() != expires {
		t.Fatalf("unexpected reserve mapping: response=%+v request=%+v err=%v", r, store.got, err)
	}
	c, err := server.Confirm(context.Background(), &stockpb.ReservationKey{ReservationId: "order-1", CustomerId: "customer-1"})
	if err != nil || c.Status != reservation.Confirmed {
		t.Fatalf("confirm %v %v", c, err)
	}
}

func TestReservationRPCRejectsMalformedCart(t *testing.T) {
	server := NewGRPCServer(app.Application{Reservations: &reservationStub{}})
	_, err := server.Reserve(context.Background(), &stockpb.ReserveRequest{ReservationId: "order-1", CustomerId: "customer-1", Items: []*orderpb.ItemWithQuantity{nil}, ExpiresAtUnixMillis: time.Now().Add(time.Hour).UnixMilli()})
	if err == nil {
		t.Fatal("nil SKU accepted")
	}
}
