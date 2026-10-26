package adapters

import (
	"context"
	"sync"

	"github.com/google/uuid"
	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/sirupsen/logrus"
)

type MemoryOrderRepository struct {
	lock  *sync.RWMutex
	store []*domain.Order
}

func NewMemoryOrderRepository() *MemoryOrderRepository {
	s := make([]*domain.Order, 0)
	s = append(s, &domain.Order{
		ID:          "fake-id",
		CustomerID:  "fake-customer-id",
		Status:      "fake-order-status",
		PaymentLink: "fake-payment-link",
		Items:       nil,
	})

	return &MemoryOrderRepository{
		lock:  &sync.RWMutex{},
		store: s,
	}
}

func (m *MemoryOrderRepository) Create(ctx context.Context, o *domain.Order) (*domain.Order, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	newOrder := &domain.Order{
		ID:          uuid.New().String(),
		CustomerID:  o.CustomerID,
		Status:      o.Status,
		PaymentLink: o.PaymentLink,
		Items:       o.Items,
	}

	m.store = append(m.store, newOrder)
	logrus.WithFields(
		logrus.Fields{
			"input_order":        o,
			"store_after_create": m.store,
		}).Info("memory order repo Create new order")
	return newOrder, nil
}

func (m *MemoryOrderRepository) Get(ctx context.Context, id, customerID string) (*domain.Order, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	for _, order := range m.store {
		if order.ID == id && order.CustomerID == customerID {
			logrus.Infof("memory_order_repo_get || found || id %s || customerID %s || result = %+v ", id, order.CustomerID, *order)
			return order, nil
		}
	}
	return nil, domain.NotFoundError{OrderID: id}
}

func (m *MemoryOrderRepository) AttachPaymentSession(
	_ context.Context,
	orderID, customerID, paymentLink string,
) (bool, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, current := range m.store {
		if current.ID != orderID || current.CustomerID != customerID {
			continue
		}
		if current.Status == domain.StatusWaitingForPayment && current.PaymentLink == paymentLink {
			return false, nil
		}
		if current.Status != domain.StatusPending {
			return false, domain.InvalidStatusTransitionError{
				OrderID:       orderID,
				CurrentStatus: current.Status,
				TargetStatus:  domain.StatusWaitingForPayment,
			}
		}
		current.Status = domain.StatusWaitingForPayment
		current.PaymentLink = paymentLink
		return true, nil
	}

	return false, domain.NotFoundError{OrderID: orderID}
}

func (m *MemoryOrderRepository) MarkPaid(
	_ context.Context,
	orderID, customerID, paymentLink string,
) (bool, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, current := range m.store {
		if current.ID != orderID || current.CustomerID != customerID {
			continue
		}
		if current.Status == domain.StatusPaid {
			return false, nil
		}
		if current.Status != domain.StatusWaitingForPayment {
			return false, domain.InvalidStatusTransitionError{
				OrderID:       orderID,
				CurrentStatus: current.Status,
				TargetStatus:  domain.StatusPaid,
			}
		}
		current.Status = domain.StatusPaid
		if paymentLink != "" {
			current.PaymentLink = paymentLink
		}
		return true, nil
	}

	return false, domain.NotFoundError{OrderID: orderID}
}

func (m *MemoryOrderRepository) MarkReady(
	_ context.Context,
	orderID, customerID string,
) (bool, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, current := range m.store {
		if current.ID != orderID || current.CustomerID != customerID {
			continue
		}
		if current.Status == domain.StatusReady {
			return false, nil
		}
		if current.Status != domain.StatusPaid {
			return false, domain.InvalidStatusTransitionError{
				OrderID:       orderID,
				CurrentStatus: current.Status,
				TargetStatus:  domain.StatusReady,
			}
		}
		current.Status = domain.StatusReady
		return true, nil
	}

	return false, domain.NotFoundError{OrderID: orderID}
}
