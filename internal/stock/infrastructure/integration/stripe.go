package integration

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/product"
)

type StripeAPI struct {
	apiKey string
}

func NewStripeAPI() *StripeAPI {
	key := viper.GetString("stripe-key")
	if key == "" {
		logrus.Fatal("stripe-key is nil")
	}
	return &StripeAPI{apiKey: key}
}

func (s *StripeAPI) GetPriceByProductID(ctx context.Context, productID string) (string, error) {
	// Log...
	stripe.Key = s.apiKey
	//logrus.Infof("apikey:%s product id: %s", s.apiKey, productID)
	params := &stripe.ProductParams{}
	params.Context = ctx
	result, err := product.Get(productID, params)
	if err != nil {
		return "", err
	}
	if result == nil || result.DefaultPrice == nil || result.DefaultPrice.ID == "" {
		return "", fmt.Errorf("product %s has no default price", productID)
	}
	return result.DefaultPrice.ID, nil
}
