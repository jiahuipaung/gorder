package app

import (
	"github.com/jiahuipaung/gorder/order/app/command"
	"github.com/jiahuipaung/gorder/order/app/query"
)

type Application struct {
	Commands Commands
	Queries  Queries
}

type Commands struct {
	CreateOrder          command.CreateOrderHandler
	AttachPaymentSession command.AttachPaymentSessionHandler
	MarkOrderPaid        command.MarkOrderPaidHandler
	MarkOrderReady       command.MarkOrderReadyHandler
}

type Queries struct {
	GetCustomerOrder query.GetCustomerOrderHandler
}
