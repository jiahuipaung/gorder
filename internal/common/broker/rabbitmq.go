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
	err = ch.ExchangeDeclare(EventOrderCreate, "direct", true, false, false, false, nil)
	if err != nil {
		logrus.Fatalf("failed to declare EventOrderCreate: %s", err)
	}
	err = ch.ExchangeDeclare(EventOrderPaid, "fanout", true, false, false, false, nil)
	if err != nil {
		logrus.Fatalf("failed to declare EventOrderPaid: %s", err)
	}
	if err = createDLX(ch); err != nil {
		logrus.Fatalf("failed to create DLX: %s", err)
	}
	return ch, conn.Close
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

func HandleRetry(ctx context.Context, ch *amqp.Channel, d *amqp.Delivery) error {
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
		return ch.PublishWithContext(ctx, "", DLQ, false, false, amqp.Publishing{
			Headers:      d.Headers,
			ContentType:  "application/json",
			Body:         d.Body,
			DeliveryMode: amqp.Persistent,
		})
	}
	logrus.Infof("retring message %s, count=%s", d.MessageId, retryCount)
	time.Sleep(time.Duration(retryCount) * time.Second)
	return ch.PublishWithContext(ctx, d.Exchange, d.RoutingKey, false, false, amqp.Publishing{
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
