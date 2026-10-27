package integration

import (
	"context"
	"github.com/stripe/stripe-go/v81"
	"io"
	"net/http"
	"strings"
	"testing"
)

type productTransport struct{ body string }

func (p productTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(p.body)), Request: r}, nil
}

func TestProductWithoutDefaultPriceReturnsError(t *testing.T) {
	previous := stripe.GetBackend(stripe.APIBackend)
	defer stripe.SetBackend(stripe.APIBackend, previous)
	for _, body := range []string{`{"id":"prod_a","object":"product","default_price":null}`, `{"id":"prod_a","object":"product","default_price":{"id":""}}`} {
		stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{HTTPClient: &http.Client{Transport: productTransport{body: body}}}))
		api := &StripeAPI{apiKey: "sk_test_fixture"}
		if _, err := api.GetPriceByProductID(context.Background(), "prod_a"); err == nil {
			t.Fatal("missing default price accepted")
		}
	}
}
