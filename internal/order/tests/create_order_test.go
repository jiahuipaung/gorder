package tests

import (
	"context"
	"fmt"
	sw "github.com/jiahuipaung/gorder/common/client/order"
	_ "github.com/jiahuipaung/gorder/common/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"log"
	"net/http"
	"testing"
)

var (
	server = fmt.Sprintf("http://%s/api", viper.Sub("order").GetString("http-addr"))
	ctx    = context.Background()
)

func TestMain(m *testing.M) {
	before()
	m.Run()
}

func before() {
	log.Printf("server starting at %s", server)
}

func getResponse(t *testing.T, customerID string, body sw.PostCustomerCustomerIdOrdersJSONRequestBody) (*sw.PostCustomerCustomerIdOrdersResponse, error) {
	t.Helper()
	client, err := sw.NewClientWithResponses(server)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.PostCustomerCustomerIdOrdersWithResponse(ctx, customerID, body)
	return response, err
}

func TestCreateOrder_success(t *testing.T) {
	response, err := getResponse(t, "123", sw.PostCustomerCustomerIdOrdersJSONRequestBody{
		CustomerId: "123",
		Items: []sw.ItemWithQuantity{
			{
				Id:       "test-item-1",
				Quantity: 1,
			},
			{
				Id:       "test-item-2",
				Quantity: 30,
			},
		},
	})
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, http.StatusOK, response.StatusCode())
	assert.Equal(t, 0, response.JSON200.Errno)
}

func TestCreateOrder_invalidParams(t *testing.T) {
	response, err := getResponse(t, "123", sw.PostCustomerCustomerIdOrdersJSONRequestBody{
		CustomerId: "123",
		Items:      nil,
	})
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, http.StatusOK, response.StatusCode())
	assert.Equal(t, 2, response.JSON200.Errno)
}
