package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/common/tracing"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
)

type StripeProcessor struct {
	apiKey string
}

func NewStripePrcessor(apiKey string) *StripeProcessor {
	if apiKey == "" {
		panic("api key is empty")
	}
	stripe.Key = apiKey
	return &StripeProcessor{apiKey: apiKey}
}

const (
	successURL = "http://localhost:8282/success/"
)

func (s StripeProcessor) CreatePaymentLink(ctx context.Context, order *orderpb.Order) (string, error) {
	_, span := tracing.Start(ctx, "stripe_processor.create_paymentLink")
	defer span.End()
	result, err := session.New(newCheckoutSessionParams(order))
	if err != nil {
		return "", err
	}
	return result.URL, nil
}

func newCheckoutSessionParams(order *orderpb.Order) *stripe.CheckoutSessionParams {
	var items []*stripe.CheckoutSessionLineItemParams
	for _, item := range order.Items {
		items = append(items, &stripe.CheckoutSessionLineItemParams{
			Price:    stripe.String(item.PriceID),
			Quantity: stripe.Int64(int64(item.Quantity)),
		})
	}
	marshalledItems, _ := json.Marshal(order.Items)
	metadata := map[string]string{
		"orderID":     order.ID,
		"customerID":  order.CustomerID,
		"status":      order.Status,
		"items":       string(marshalledItems),
		"paymentLink": order.PaymentLink,
	}

	params := &stripe.CheckoutSessionParams{
		Metadata:   metadata,
		LineItems:  items,
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(fmt.Sprintf("%s?customerID=%s&orderID=%s", successURL, order.CustomerID, order.ID)),
	}
	params.SetIdempotencyKey(order.ID)
	return params
}
