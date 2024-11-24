package order

import (
	"errors"
	"fmt"
	"github.com/jiahuipaung/gorder/order/entity"

	"github.com/stripe/stripe-go/v81"
)

// Aggregate聚合
type Order struct {
	ID          string
	CustomerID  string
	Status      string
	PaymentLink string
	Items       []*entity.Item
}

func NewOrder(customerID, ID, status, paymentLink string, items []*entity.Item) (*Order, error) {
	if customerID == "" {
		return nil, errors.New("customerID is nil")
	}
	if ID == "" {
		return nil, errors.New("ID is nil")
	}
	if status == "" {
		return nil, errors.New("status is nil")
	}
	if items == nil {
		return nil, errors.New("items is nil")
	}
	return &Order{
		CustomerID:  customerID,
		ID:          ID,
		Status:      status,
		PaymentLink: paymentLink,
		Items:       items,
	}, nil
}

func (o Order) IsPaid() error {
	if o.Status == string(stripe.CheckoutSessionPaymentStatusPaid) {
		return nil
	}
	return fmt.Errorf("order %s is not paid", o.ID)
}
