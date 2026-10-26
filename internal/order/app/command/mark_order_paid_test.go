package command

import (
	"context"
	"testing"

	"github.com/jiahuipaung/gorder/common/metrics"
	"github.com/jiahuipaung/gorder/order/adapters"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

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
