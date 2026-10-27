package command

import (
	"context"
	"errors"
	"testing"

	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
	"github.com/jiahuipaung/gorder/common/metrics"
	"github.com/jiahuipaung/gorder/order/adapters"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

type reservationVerifierStub struct {
	status string
	err    error
}

func (s reservationVerifierStub) GetReservation(_ context.Context, id, owner string) (*stockpb.ReservationResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	state := s.status
	if state == "" {
		state = "CONFIRMED"
	}
	return &stockpb.ReservationResponse{ReservationId: id, CustomerId: owner, Status: state}, nil
}

func TestMarkOrderPaidHandlerTransitionsOnceAndTreatsReplayAsNoOp(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := adapters.NewMemoryOrderRepository()
	created, err := repo.Create(ctx, &domain.Order{
		CustomerID: "customer-1",
		Status:     domain.StatusWaitingForPayment,
	})
	require.NoError(t, err)

	handler := NewMarkOrderPaidHandler(
		repo,
		reservationVerifierStub{},
		logrus.NewEntry(logrus.New()),
		metrics.TodoMetrics{},
	)
	cmd := MarkOrderPaid{Order: &domain.Order{
		ID:          created.ID,
		CustomerID:  created.CustomerID,
		Status:      domain.StatusPaid,
		PaymentLink: "payment-link-1",
	}}

	changed, err := handler.Handler(ctx, cmd)
	require.NoError(t, err)
	require.True(t, changed)

	changed, err = handler.Handler(ctx, cmd)
	require.NoError(t, err)
	require.False(t, changed)
}

func TestMarkOrderPaidHandlerRejectsNilOrder(t *testing.T) {
	t.Parallel()

	handler := NewMarkOrderPaidHandler(
		adapters.NewMemoryOrderRepository(),
		reservationVerifierStub{},
		logrus.NewEntry(logrus.New()),
		metrics.TodoMetrics{},
	)

	changed, err := handler.Handler(context.Background(), MarkOrderPaid{})
	require.Error(t, err)
	require.False(t, changed)
}

func TestMarkOrderPaidHandlerRejectsNonPaidEventWithoutChangingOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := adapters.NewMemoryOrderRepository()
	created, err := repo.Create(ctx, &domain.Order{
		CustomerID: "customer-1",
		Status:     domain.StatusWaitingForPayment,
	})
	require.NoError(t, err)
	handler := NewMarkOrderPaidHandler(
		repo,
		reservationVerifierStub{},
		logrus.NewEntry(logrus.New()),
		metrics.TodoMetrics{},
	)

	changed, err := handler.Handler(ctx, MarkOrderPaid{Order: &domain.Order{
		ID:          created.ID,
		CustomerID:  created.CustomerID,
		Status:      domain.StatusWaitingForPayment,
		PaymentLink: "must-not-be-written",
	}})
	require.Error(t, err)
	require.False(t, changed)

	got, err := repo.Get(ctx, created.ID, created.CustomerID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusWaitingForPayment, got.Status)
	require.Empty(t, got.PaymentLink)
}

func TestMarkOrderPaidRequiresConfirmedReservation(t *testing.T) {
	ctx := context.Background()
	repo := adapters.NewMemoryOrderRepository()
	o, err := repo.Create(ctx, &domain.Order{CustomerID: "customer", Status: domain.StatusWaitingForPayment})
	require.NoError(t, err)
	h := NewMarkOrderPaidHandler(repo, reservationVerifierStub{status: "RELEASED"}, logrus.NewEntry(logrus.New()), metrics.TodoMetrics{})
	changed, err := h.Handler(ctx, MarkOrderPaid{Order: &domain.Order{ID: o.ID, CustomerID: "customer", Status: domain.StatusPaid}})
	require.Error(t, err)
	require.False(t, changed)
	stored, err := repo.Get(ctx, o.ID, "customer")
	require.NoError(t, err)
	require.Equal(t, domain.StatusWaitingForPayment, stored.Status)
	h = NewMarkOrderPaidHandler(repo, reservationVerifierStub{err: errors.New("stock unavailable")}, logrus.NewEntry(logrus.New()), metrics.TodoMetrics{})
	_, err = h.Handler(ctx, MarkOrderPaid{Order: &domain.Order{ID: o.ID, CustomerID: "customer", Status: domain.StatusPaid}})
	require.Error(t, err)
}
