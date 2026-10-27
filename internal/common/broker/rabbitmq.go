package broker

import (
	"context"
	"fmt"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"time"

	_ "github.com/jiahuipaung/gorder/common/config"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

const (
	DLX                = "order_dlx"
	DLQ                = "order_dlq"
	OrderPaidQueue     = "order.paid.order"
	KitchenPaidQueue   = "order.paid.kitchen"
	amqpRetryHeaderKey = "x-retry-count"
)

var (
	maxRetryCount = viper.Sub("rabbit-mq").GetInt64("max-retry-count")
)

func Connect(user, password, host, port string) (*amqp.Channel, func() error) {
	address := fmt.Sprintf("amqp://%s:%s@%s:%s/", user, password, host, port)
	conn, err := amqp.Dial(address)
	if err != nil {
		logrus.Fatalf("failed to connect to RabbitMQ: %s", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		logrus.Fatalf("failed to open a channel: %s", err)
	}
	if err = DeclareTopology(ch); err != nil {
		logrus.Fatalf("failed to declare message topology: %s", err)
	}
	return ch, conn.Close
}

// Declare every durable subscription before any publisher can accept a payment.
func DeclareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(EventOrderCreate, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(EventOrderPaid, "fanout", true, false, false, false, nil); err != nil {
		return err
	}
	for _, name := range []string{EventOrderCreate, OrderPaidQueue, KitchenPaidQueue} {
		if _, err := ch.QueueDeclare(name, true, false, false, false, nil); err != nil {
			return err
		}
	}
	for _, name := range []string{OrderPaidQueue, KitchenPaidQueue} {
		if err := ch.QueueBind(name, "", EventOrderPaid, false, nil); err != nil {
			return err
		}
	}
	return createDLX(ch)
}

func createDLX(ch *amqp.Channel) error {
	q, err := ch.QueueDeclare("share_queue", true, false, false, false, nil)
	if err != nil {
		return err
	}
	err = ch.ExchangeDeclare(DLX, "fanout", true, false, false, false, nil)
	if err != nil {
		return err
	}
	err = ch.QueueBind(q.Name, "", DLX, false, nil)
	if err != nil {
		return err
	}
	_, err = ch.QueueDeclare(DLQ, true, false, false, false, nil)
	if err != nil {
		return err
	}
	return nil
}

func HandleRetryToQueue(ctx context.Context, ch *amqp.Channel, d *amqp.Delivery, queue string) error {
	if queue == "" {
		return fmt.Errorf("retry queue is required")
	}
	publisher, err := ConfirmedPublisherFor(ch)
	if err != nil {
		return err
	}
	if d.Headers == nil {
		d.Headers = amqp.Table{}
	}
	retryCount, ok := d.Headers[amqpRetryHeaderKey].(int64)
	if !ok {
		retryCount = 0
	}
	retryCount++
	d.Headers[amqpRetryHeaderKey] = retryCount

	if retryCount > maxRetryCount {
		logrus.Infof("moving message %s to dlq", d.MessageId)
		return publisher.PublishWithContext(ctx, "", DLQ, true, false, amqp.Publishing{
			Headers:      d.Headers,
			ContentType:  "application/json",
			Body:         d.Body,
			DeliveryMode: amqp.Persistent,
		})
	}
	logrus.Infof("retrying message %s, count=%d", d.MessageId, retryCount)
	timer := time.NewTimer(time.Duration(retryCount) * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	return publisher.PublishWithContext(ctx, "", queue, true, false, amqp.Publishing{
		Headers:      d.Headers,
		ContentType:  "application/json",
		Body:         d.Body,
		DeliveryMode: amqp.Persistent,
	})
}

type RabbitMQHeaderCarrier map[string]interface {
}

func (r RabbitMQHeaderCarrier) Get(key string) string {
	value, ok := r[key]
	if !ok {
		return ""
	}
	return value.(string)
}

func (r RabbitMQHeaderCarrier) Set(key string, value string) {
	r[key] = value

}

func (r RabbitMQHeaderCarrier) Keys() []string {
	keys := make([]string, len(r))
	i := 0
	for k := range r {
		keys[i] = k
		i++
	}
	return keys
}

func InjectRabbitMQHeader(ctx context.Context) map[string]interface{} {
	carrier := make(RabbitMQHeaderCarrier)
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier
}

func ExtractRabbitMQHeader(ctx context.Context, headers map[string]interface{}) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, RabbitMQHeaderCarrier(headers))
}
