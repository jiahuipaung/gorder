package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/otel"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jiahuipaung/gorder/common/broker"
	"github.com/jiahuipaung/gorder/common/genproto/orderpb"
	"github.com/jiahuipaung/gorder/payment/domain"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
)

type PaymentHandler struct {
	ch *amqp.Channel
}

func NewPaymentHandler(ch *amqp.Channel) *PaymentHandler {
	return &PaymentHandler{ch: ch}
}

func (h *PaymentHandler) RegisterRoutes(c *gin.Engine) {
	c.POST("/api/webhook", h.handleWebhook)
}

func (h *PaymentHandler) handleWebhook(c *gin.Context) {
	logrus.Info("handle webhook")
	const MaxBodyBytes = int64(65536)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxBodyBytes)
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logrus.Infof("Error reading request body: %v\n", err)
		c.JSON(http.StatusServiceUnavailable, err.Error())
		return
	}

	event, err := webhook.ConstructEvent(payload, c.Request.Header.Get("Stripe-Signature"),
		viper.GetString("endpoint-stripe-secret"))

	if err != nil {
		logrus.Infof("Error verifying webhook signature: %v\n", err)
		c.JSON(http.StatusBadRequest, err.Error()) // Return a 400 error on a bad signature
		return
	}

	// Unmarshal the event data into an appropriate struct depending on its Type
	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted:
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			logrus.Infof("Error unmarshaling event.data.raw into session: %v\n", err)
			c.JSON(http.StatusBadRequest, err.Error())
			return
		}
		if session.PaymentStatus == stripe.CheckoutSessionPaymentStatusPaid {
			logrus.Infof("Payment for checksession %v is Paid\n", session.ID)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var items []*orderpb.Item
			_ = json.Unmarshal([]byte(session.Metadata["items"]), &items)
			marshalledOrder, err := json.Marshal(&domain.Order{
				ID:          session.Metadata["orderID"],
				Status:      string(stripe.CheckoutSessionPaymentStatusPaid),
				CustomerID:  session.Metadata["customerID"],
				Items:       items,
				PaymentLink: session.Metadata["paymentLink"],
			})
			if err != nil {
				logrus.Infof("Error marshalling orders: %v\n", err)
				c.JSON(http.StatusBadRequest, err.Error())
				return
			}
			t := otel.Tracer("rabbitMQ")
			mqCtx, span := t.Start(ctx, fmt.Sprintf("rabbitMQ.%s.publish", broker.EventOrderPaid))
			defer span.End()
			headers := broker.InjectRabbitMQHeader(mqCtx)
			_ = h.ch.PublishWithContext(mqCtx, broker.EventOrderPaid, "", false, false,
				amqp.Publishing{
					ContentType:  "application/json",
					Body:         marshalledOrder,
					DeliveryMode: amqp.Persistent,
					Headers:      headers,
				})
			logrus.Infof("messgae published to %s , body :%s\n", broker.EventOrderPaid, string(marshalledOrder))
		}
	}
	c.JSON(http.StatusOK, nil)
}
