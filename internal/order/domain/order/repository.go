package order

import (
	"context"
	"fmt"
)

type Repository interface {
	Create(context.Context, *Order) (*Order, error)
	Get(ctx context.Context, id, customerID string) (*Order, error)
	MarkPaid(ctx context.Context, orderID, customerID, paymentLink string) (bool, error)
	Update(ctx context.Context,
		o *Order,
		updateFn func(context.Context, *Order) (*Order, error),
	) error
}

type InvalidStatusTransitionError struct {
	OrderID       string
	CurrentStatus string
	TargetStatus  string
}

func (e InvalidStatusTransitionError) Error() string {
	return fmt.Sprintf(
		"order %s cannot transition from %s to %s",
		e.OrderID,
		e.CurrentStatus,
		e.TargetStatus,
	)
}

type NotFoundError struct {
	OrderID string
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("order with id %s not found", e.OrderID)
}
