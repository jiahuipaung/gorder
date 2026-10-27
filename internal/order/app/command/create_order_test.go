package command

import (
	"testing"

	domain "github.com/jiahuipaung/gorder/order/domain/order"
	"github.com/jiahuipaung/gorder/order/entity"
	"github.com/stretchr/testify/require"
)

func TestNewOrderForCreationStartsPending(t *testing.T) {
	t.Parallel()

	items := []*entity.Item{{ID: "item-1", Quantity: 2}}
	order := newOrderForCreation("customer-1", items)

	require.Equal(t, "customer-1", order.CustomerID)
	require.Equal(t, domain.StatusPending, order.Status)
	require.Same(t, items[0], order.Items[0])
}
