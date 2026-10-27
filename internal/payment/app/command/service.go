package command

import "context"

type OrderService interface {
	AttachPaymentSession(ctx context.Context, orderID, customerID, paymentLink string) error
}
