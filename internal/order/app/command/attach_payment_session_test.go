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

func TestAttachPaymentSessionHandlerTransitionsOnceAndTreatsExactReplayAsNoOp(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := adapters.NewMemoryOrderRepository()
	created, err := repo.Create(ctx, &domain.Order{
		CustomerID: "customer-1",
		Status:     domain.StatusPending,
	})
	require.NoError(t, err)
	handler := NewAttachPaymentSessionHandler(
		repo,
		logrus.NewEntry(logrus.New()),
		metrics.TodoMetrics{},
	)
	cmd := AttachPaymentSession{
		OrderID:     created.ID,
		CustomerID:  created.CustomerID,
		PaymentLink: "checkout-link-1",
	}

	changed, err := handler.Handler(ctx, cmd)
	require.NoError(t, err)
	require.True(t, changed)

	changed, err = handler.Handler(ctx, cmd)
	require.NoError(t, err)
	require.False(t, changed)
}

func TestAttachPaymentSessionHandlerRejectsEmptyFields(t *testing.T) {
	t.Parallel()

	handler := NewAttachPaymentSessionHandler(
		adapters.NewMemoryOrderRepository(),
		logrus.NewEntry(logrus.New()),
		metrics.TodoMetrics{},
	)
	tests := []struct {
		name string
		cmd  AttachPaymentSession
	}{
		{name: "order id", cmd: AttachPaymentSession{CustomerID: "customer-1", PaymentLink: "link"}},
		{name: "customer id", cmd: AttachPaymentSession{OrderID: "order-1", PaymentLink: "link"}},
		{name: "payment link", cmd: AttachPaymentSession{OrderID: "order-1", CustomerID: "customer-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed, err := handler.Handler(context.Background(), tt.cmd)
			require.Error(t, err)
			require.False(t, changed)
		})
	}
}

func TestAttachPaymentSessionHandlerPropagatesRepositoryClassification(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := adapters.NewMemoryOrderRepository()
	created, err := repo.Create(ctx, &domain.Order{
		CustomerID: "customer-1",
		Status:     domain.StatusPaid,
	})
	require.NoError(t, err)
	handler := NewAttachPaymentSessionHandler(
		repo,
		logrus.NewEntry(logrus.New()),
		metrics.TodoMetrics{},
	)

	changed, err := handler.Handler(ctx, AttachPaymentSession{
		OrderID:     created.ID,
		CustomerID:  created.CustomerID,
		PaymentLink: "checkout-link-1",
	})
	require.False(t, changed)
	var invalid domain.InvalidStatusTransitionError
	require.ErrorAs(t, err, &invalid)

	changed, err = handler.Handler(ctx, AttachPaymentSession{
		OrderID:     "missing-order",
		CustomerID:  "customer-1",
		PaymentLink: "checkout-link-1",
	})
	require.False(t, changed)
	var notFound domain.NotFoundError
	require.ErrorAs(t, err, &notFound)
}
