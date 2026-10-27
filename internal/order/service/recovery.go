package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jiahuipaung/gorder/order/app/command"
	"github.com/jiahuipaung/gorder/order/app/query"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PendingRepository interface {
	ListPending(context.Context, string, int) ([]*domain.Order, error)
	MarkCreatePublished(context.Context, string, string) error
	RejectStock(context.Context, string, string) error
}

// RecoverPendingOnce scans every current page. A failed intent remains pending
// so the next pass can retry the same stable reservation and payment request.
func RecoverPendingOnce(ctx context.Context, repo PendingRepository, stock query.StockService, publisher command.OrderPublisher) error {
	const pageSize = 100
	var after string
	var failures []error
	for {
		orders, err := repo.ListPending(ctx, after, pageSize)
		if err != nil {
			return errors.Join(append(failures, err)...)
		}
		for _, order := range orders {
			if err := command.EnsurePending(ctx, order, stock, publisher); err != nil {
				if status.Code(err) == codes.ResourceExhausted {
					if rejectErr := repo.RejectStock(ctx, order.ID, order.CustomerID); rejectErr != nil {
						failures = append(failures, fmt.Errorf("reject order %s: %w", order.ID, rejectErr))
					}
				} else {
					failures = append(failures, fmt.Errorf("order %s: %w", order.ID, err))
				}
			} else if err := repo.MarkCreatePublished(ctx, order.ID, order.CustomerID); err != nil {
				failures = append(failures, fmt.Errorf("mark order %s published: %w", order.ID, err))
			}
			after = order.ID
		}
		if len(orders) < pageSize {
			return errors.Join(failures...)
		}
	}
}

func runPendingRecovery(ctx context.Context, repo PendingRepository, stock query.StockService, publisher command.OrderPublisher) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		passCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := RecoverPendingOnce(passCtx, repo, stock, publisher); err != nil {
			logrus.Warnf("pending order recovery: %v", err)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
