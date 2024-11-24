package processor

import (
	"context"

	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
)

type InMemProcessor struct {
}

func NewInMemProcessor() *InMemProcessor {
	return &InMemProcessor{}
}

func (i InMemProcessor) CreatePaymentLink(ctx context.Context, order *orderpb.Order) (string, error) {
	return "inmem-payment-link", nil
}
