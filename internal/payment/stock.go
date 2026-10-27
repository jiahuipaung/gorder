package main

import (
	"context"
	"fmt"

	"github.com/jiahuipaung/gorder/common/genproto/stockpb"
)

type grpcReservationConfirmer struct{ client stockpb.StockServiceClient }

func (c grpcReservationConfirmer) Confirm(ctx context.Context, id, customer string) error {
	r, err := c.client.Confirm(ctx, &stockpb.ReservationKey{ReservationId: id, CustomerId: customer})
	if err != nil {
		return err
	}
	if r == nil || r.ReservationId != id || r.CustomerId != customer || r.Status != "CONFIRMED" {
		return fmt.Errorf("stock returned inconsistent reservation confirmation")
	}
	return nil
}
