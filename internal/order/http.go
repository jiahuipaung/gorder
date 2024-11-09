package main

import (
	"github.com/gin-gonic/gin"
	"github.com/jiahuipaung/gorder/order/app"
	"github.com/jiahuipaung/gorder/order/app/query"
	"net/http"
)

type HTTPServer struct {
	app app.Application
}

func (s *HTTPServer) PostCustomerCustomerIDOrders(c *gin.Context, customerID string) {

}

func (s *HTTPServer) GetCustomerCustomerIDOrdersOrderID(c *gin.Context, customerID string, orderID string) {
	o, err := s.app.Queries.GetCustomerOrder.Handler(c, query.GetCustomerOrder{
		OrderId:    "fake-id",
		CustomerId: "fake-customer-id",
	})

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": o})

}
