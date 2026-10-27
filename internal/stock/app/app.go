package app

import (
	"context"
	"github.com/jiahuipaung/gorder/stock/app/query"
	"github.com/jiahuipaung/gorder/stock/reservation"
)

type Application struct {
	Commands     Commands
	Queries      Queries
	Reservations ReservationService
}

type ReservationService interface {
	Reserve(context.Context, reservation.Request) (*reservation.Reservation, error)
	Confirm(context.Context, string, string) (*reservation.Reservation, error)
	Release(context.Context, string, string) (*reservation.Reservation, error)
	Get(context.Context, string, string) (*reservation.Reservation, error)
}

type Commands struct {
}

type Queries struct {
	CheckIfItemsInStock query.CheckIfItemsInStockHandler
	GetItems            query.GetItemsHandler
}
