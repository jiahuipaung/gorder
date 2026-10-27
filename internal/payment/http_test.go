package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jiahuipaung/gorder/payment/domain"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/spf13/viper"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
)

type publishCall struct {
	exchange   string
	routingKey string
	mandatory  bool
	immediate  bool
	message    amqp.Publishing
}

type publisherStub struct {
	err   error
	calls []publishCall
}

func (s *publisherStub) PublishWithContext(
	_ context.Context,
	exchange string,
	routingKey string,
	mandatory bool,
	immediate bool,
	message amqp.Publishing,
) error {
	s.calls = append(s.calls, publishCall{
		exchange:   exchange,
		routingKey: routingKey,
		mandatory:  mandatory,
		immediate:  immediate,
		message:    message,
	})
	return s.err
}

func TestPaidWebhookReturnsOKAfterPublishingOrderPaid(t *testing.T) {
	publisher := &publisherStub{}
	status := serveSignedPaidWebhook(t, publisher, true)

	if status != http.StatusOK {
		t.Fatalf("expected HTTP %d, got %d", http.StatusOK, status)
	}
	if len(publisher.calls) != 1 {
		t.Fatalf("expected one publish, got %d", len(publisher.calls))
	}
	call := publisher.calls[0]
	if call.exchange != "order.paid" || call.routingKey != "" {
		t.Fatalf("expected order.paid exchange with empty routing key, got exchange=%q routingKey=%q", call.exchange, call.routingKey)
	}
	if call.message.ContentType != "application/json" || call.message.DeliveryMode != amqp.Persistent {
		t.Fatalf("unexpected AMQP publication properties: %+v", call.message)
	}
	var order domain.Order
	if err := json.Unmarshal(call.message.Body, &order); err != nil {
		t.Fatalf("decode published order: %v", err)
	}
	if order.ID != "order-1" || order.CustomerID != "customer-1" || order.Status != "paid" || order.PaymentLink != "https://pay.example/session-1" {
		t.Fatalf("unexpected published order: %+v", order)
	}
	if len(order.Items) != 1 || order.Items[0].ID != "item-1" || order.Items[0].Quantity != 2 {
		t.Fatalf("unexpected published items: %+v", order.Items)
	}
}

func TestPaidWebhookReturnsServiceUnavailableWhenPublishFails(t *testing.T) {
	publisher := &publisherStub{err: errors.New("rabbitmq unavailable")}
	status := serveSignedPaidWebhook(t, publisher, true)

	if status != http.StatusServiceUnavailable {
		t.Fatalf("expected HTTP %d, got %d", http.StatusServiceUnavailable, status)
	}
	if len(publisher.calls) != 1 {
		t.Fatalf("expected one publish attempt, got %d", len(publisher.calls))
	}
}

func TestWebhookRejectsInvalidSignatureWithoutPublishing(t *testing.T) {
	publisher := &publisherStub{}
	status := serveSignedPaidWebhook(t, publisher, false)

	if status != http.StatusBadRequest {
		t.Fatalf("expected HTTP %d, got %d", http.StatusBadRequest, status)
	}
	if len(publisher.calls) != 0 {
		t.Fatalf("expected no publish attempt, got %d", len(publisher.calls))
	}
}

func serveSignedPaidWebhook(t *testing.T, publisher *publisherStub, validSignature bool) int {
	t.Helper()

	const secret = "whsec_consumer_correctness_test"
	previousSecret := viper.Get("endpoint-stripe-secret")
	viper.Set("endpoint-stripe-secret", secret)
	t.Cleanup(func() {
		viper.Set("endpoint-stripe-secret", previousSecret)
	})

	items, err := json.Marshal([]map[string]any{{"id": "item-1", "quantity": 2}})
	if err != nil {
		t.Fatalf("marshal items fixture: %v", err)
	}
	payload, err := json.Marshal(map[string]any{
		"id":          "evt_paid_1",
		"object":      "event",
		"api_version": stripe.APIVersion,
		"type":        stripe.EventTypeCheckoutSessionCompleted,
		"data": map[string]any{
			"object": map[string]any{
				"id":             "cs_test_1",
				"object":         "checkout.session",
				"payment_status": "paid",
				"metadata": map[string]string{
					"orderID":     "order-1",
					"customerID":  "customer-1",
					"items":       string(items),
					"paymentLink": "https://pay.example/session-1",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal Stripe event fixture: %v", err)
	}
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  secret,
	})
	signature := signed.Header
	if !validSignature {
		signature = "t=1,v1=invalid"
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewPaymentHandler(publisher).RegisterRoutes(router)
	req := httptest.NewRequest(http.MethodPost, "/api/webhook", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", signature)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	return recorder.Code
}
