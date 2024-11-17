package order

import (
	"errors"
	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
)

type Order struct {
	ID          string
	CustomerID  string
	Status      string
	PaymentLink string
	Items       []*orderpb.Item
}

func NewOrder(customerID, ID, status, paymentLink string, items []*orderpb.Item) (*Order, error) {
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

func (o *Order) ToProto() *orderpb.Order {
	return &orderpb.Order{
		ID:          o.ID,
		CustomerID:  o.CustomerID,
		Status:      o.Status,
		PaymentLink: o.PaymentLink,
		Items:       o.Items,
	}
}
