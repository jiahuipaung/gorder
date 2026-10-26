package adapters

import (
	"context"
	"errors"
	"sync"
	"testing"

	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/stretchr/testify/require"
)

func TestMemoryOrderRepositoryMarkPaidTransitionsWaitingOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := NewMemoryOrderRepository()
	created, err := repo.Create(ctx, &domain.Order{
		CustomerID: "customer-1",
		Status:     domain.StatusWaitingForPayment,
	})
	require.NoError(t, err)

	changed, err := repo.MarkPaid(ctx, created.ID, created.CustomerID, "payment-link-1")
	require.NoError(t, err)
	require.True(t, changed)

	got, err := repo.Get(ctx, created.ID, created.CustomerID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusPaid, got.Status)
	require.Equal(t, "payment-link-1", got.PaymentLink)
}

func TestMemoryOrderRepositoryMarkPaidConcurrentReplayChangesOnce(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := NewMemoryOrderRepository()
	created, err := repo.Create(ctx, &domain.Order{
		CustomerID: "customer-1",
		Status:     domain.StatusWaitingForPayment,
	})
	require.NoError(t, err)

	const workers = 16
	start := make(chan struct{})
	results := make(chan bool, workers)
	errorsFound := make(chan error, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			<-start
			changed, markErr := repo.MarkPaid(
				ctx,
				created.ID,
				created.CustomerID,
				"payment-link-1",
			)
			results <- changed
			errorsFound <- markErr
		}()
	}
	close(start)
	group.Wait()
	close(results)
	close(errorsFound)

	changedCount := 0
	for changed := range results {
		if changed {
			changedCount++
		}
	}
	for markErr := range errorsFound {
		require.NoError(t, markErr)
	}
	require.Equal(t, 1, changedCount)
}

func TestMemoryOrderRepositoryMarkPaidRejectsInvalidTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		initialStatus string
		orderID       func(created *domain.Order) string
		customerID    string
		wantNotFound  bool
	}{
		{
			name:          "missing order",
			initialStatus: domain.StatusWaitingForPayment,
			orderID:       func(_ *domain.Order) string { return "missing-order" },
			customerID:    "customer-1",
			wantNotFound:  true,
		},
		{
			name:          "wrong customer",
			initialStatus: domain.StatusWaitingForPayment,
			orderID:       func(created *domain.Order) string { return created.ID },
			customerID:    "customer-2",
			wantNotFound:  true,
		},
		{
			name:          "invalid current state",
			initialStatus: "cancelled",
			orderID:       func(created *domain.Order) string { return created.ID },
			customerID:    "customer-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := NewMemoryOrderRepository()
			created, err := repo.Create(ctx, &domain.Order{
				CustomerID: "customer-1",
				Status:     tt.initialStatus,
			})
			require.NoError(t, err)

			changed, err := repo.MarkPaid(
				ctx,
				tt.orderID(created),
				tt.customerID,
				"new-payment-link",
			)
			require.False(t, changed)
			if tt.wantNotFound {
				var target domain.NotFoundError
				require.True(t, errors.As(err, &target))
			} else {
				var target domain.InvalidStatusTransitionError
				require.True(t, errors.As(err, &target))
			}

			got, getErr := repo.Get(ctx, created.ID, created.CustomerID)
			require.NoError(t, getErr)
			require.Equal(t, tt.initialStatus, got.Status)
			require.Empty(t, got.PaymentLink)
		})
	}
}

func TestMemoryOrderRepositoryMarkPaidTreatsPaidOrderAsNoOp(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := NewMemoryOrderRepository()
	created, err := repo.Create(ctx, &domain.Order{
		CustomerID: "customer-1",
		Status:     domain.StatusWaitingForPayment,
	})
	require.NoError(t, err)

	changed, err := repo.MarkPaid(ctx, created.ID, created.CustomerID, "first-link")
	require.NoError(t, err)
	require.True(t, changed)

	changed, err = repo.MarkPaid(ctx, created.ID, created.CustomerID, "replayed-link")
	require.NoError(t, err)
	require.False(t, changed)

	got, err := repo.Get(ctx, created.ID, created.CustomerID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusPaid, got.Status)
	require.Equal(t, "first-link", got.PaymentLink)
}

func TestMemoryOrderRepositoryMarkPaidPreservesStoredLinkWhenEventLinkIsEmpty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := NewMemoryOrderRepository()
	created, err := repo.Create(ctx, &domain.Order{
		CustomerID:  "customer-1",
		Status:      domain.StatusWaitingForPayment,
		PaymentLink: "stored-checkout-link",
	})
	require.NoError(t, err)

	changed, err := repo.MarkPaid(ctx, created.ID, created.CustomerID, "")
	require.NoError(t, err)
	require.True(t, changed)

	got, err := repo.Get(ctx, created.ID, created.CustomerID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusPaid, got.Status)
	require.Equal(t, "stored-checkout-link", got.PaymentLink)
}
