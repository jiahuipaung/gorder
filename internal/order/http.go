package main

import (
	"github.com/gin-gonic/gin"
	"github.com/jiahuipaung/gorder/order/app"
)

type HTTPServer struct {
	app app.Application
}

func (s *HTTPServer) PostCustomerCustomerIDOrders(c *gin.Context, customerID string) {}

func (s *HTTPServer) GetCustomerCustomerIDOrdersOrderID(c *gin.Context, customerID string, orderID string) {
}
