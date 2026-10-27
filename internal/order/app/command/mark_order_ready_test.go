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

func TestMarkOrderReadyHandlerTransitionsOnceAndTreatsReplayAsNoOp(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := adapters.NewMemoryOrderRepository()
	created, err := repo.Create(ctx, &domain.Order{
		CustomerID: "customer-1",
		Status:     domain.StatusPaid,
	})
	require.NoError(t, err)
	handler := NewMarkOrderReadyHandler(
		repo,
		logrus.NewEntry(logrus.New()),
		metrics.TodoMetrics{},
	)
	cmd := MarkOrderReady{OrderID: created.ID, CustomerID: created.CustomerID}

	changed, err := handler.Handler(ctx, cmd)
	require.NoError(t, err)
	require.True(t, changed)

	changed, err = handler.Handler(ctx, cmd)
	require.NoError(t, err)
	require.False(t, changed)
}

func TestMarkOrderReadyHandlerRejectsEmptyIdentifiers(t *testing.T) {
	t.Parallel()

	handler := NewMarkOrderReadyHandler(
		adapters.NewMemoryOrderRepository(),
		logrus.NewEntry(logrus.New()),
		metrics.TodoMetrics{},
	)
	for _, cmd := range []MarkOrderReady{
		{CustomerID: "customer-1"},
		{OrderID: "order-1"},
	} {
		changed, err := handler.Handler(context.Background(), cmd)
		require.Error(t, err)
		require.False(t, changed)
	}
}

func TestMarkOrderReadyHandlerPropagatesRepositoryClassification(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := adapters.NewMemoryOrderRepository()
	created, err := repo.Create(ctx, &domain.Order{
		CustomerID: "customer-1",
		Status:     domain.StatusPending,
	})
	require.NoError(t, err)
	handler := NewMarkOrderReadyHandler(
		repo,
		logrus.NewEntry(logrus.New()),
		metrics.TodoMetrics{},
	)

	changed, err := handler.Handler(ctx, MarkOrderReady{
		OrderID:    created.ID,
		CustomerID: created.CustomerID,
	})
	require.False(t, changed)
	var invalid domain.InvalidStatusTransitionError
	require.ErrorAs(t, err, &invalid)

	changed, err = handler.Handler(ctx, MarkOrderReady{
		OrderID:    "missing-order",
		CustomerID: "customer-1",
	})
	require.False(t, changed)
	var notFound domain.NotFoundError
	require.ErrorAs(t, err, &notFound)
}
