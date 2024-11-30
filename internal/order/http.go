package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/jiahuipaung/gorder/common"
	client "github.com/jiahuipaung/gorder/common/client/order"
	"github.com/jiahuipaung/gorder/order/app"
	"github.com/jiahuipaung/gorder/order/app/command"
	"github.com/jiahuipaung/gorder/order/app/query"
	"github.com/jiahuipaung/gorder/order/convertor"
	"order/app/dto"
)

type HTTPServer struct {
	common.BaseResponse
	app app.Application
}

func (s *HTTPServer) PostCustomerCustomerIdOrders(c *gin.Context, customerID string) {
	//ctx, span := tracing.Start(c, "PostCustomerCustomerIDOrders")
	//defer span.End()
	var (
		req  client.CreateOrderRequest
		err  error
		resp dto.CreateOrderResponse
	)
	defer func() {
		s.Response(c, err, resp)
	}()
	if err = c.ShouldBindJSON(&req); err != nil {
		return
	}
	r, err := s.app.Commands.CreateOrder.Handler(c.Request.Context(), command.CreateOrder{
		CustomerID: req.CustomerId,
		Items:      convertor.NewItemWithQuantityConverter().ClientToEntities(req.Items),
	})
	if err != nil {
		return
	}
	resp = dto.CreateOrderResponse{
		CustomerID:  req.CustomerId,
		OrderID:     r.OrderID,
		RedirectURL: fmt.Sprintf("http://localhost:8282/success?customerID=%s&orderID=%s", req.CustomerId, r.OrderID),
	}
}

func (s *HTTPServer) GetCustomerCustomerIdOrdersOrderId(c *gin.Context, customerID string, orderID string) {
	var (
		err  error
		resp struct {
			Order *client.Order `json:"order"`
		}
	)
	defer func() {
		s.Response(c, err, resp)
	}()
	o, err := s.app.Queries.GetCustomerOrder.Handler(c.Request.Context(), query.GetCustomerOrder{
		OrderId:    orderID,
		CustomerId: customerID,
	})

	if err != nil {
		return
	}
	resp.Order = convertor.NewOrderConverter().EntityToClient(o)
}
