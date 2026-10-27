//go:build integration

package broker

import (
	"context"
	amqp "github.com/rabbitmq/amqp091-go"
	"os"
	"strings"
	"testing"
	"time"
)

// Use a dedicated test vhost; these assertions inspect actual routing and queue lifetimes.
func TestPaidSubscriptionsSurviveDisconnectAndRetryIsConsumerSpecific(t *testing.T) {
	url := os.Getenv("GORDER_TEST_AMQP_URL")
	if url == "" {
		t.Fatal("GORDER_TEST_AMQP_URL must name an isolated test broker/vhost")
	}
	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	if err := DeclareTopology(ch); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{OrderPaidQueue, KitchenPaidQueue} {
		if _, err := ch.QueuePurge(q, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	conn, err = amqp.Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ch, err = conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{OrderPaidQueue, KitchenPaidQueue} {
		if _, err := ch.QueueInspect(q); err != nil {
			t.Fatalf("subscription lost on disconnect: %v", err)
		}
	}
	if err := ch.PublishWithContext(context.Background(), EventOrderPaid, "", false, false, amqp.Publishing{Body: []byte("paid"), DeliveryMode: amqp.Persistent}); err != nil {
		t.Fatal(err)
	}
	// Passive declarations act as synchronous protocol barriers after publication.
	for _, q := range []string{OrderPaidQueue, KitchenPaidQueue} {
		if _, err := ch.QueueInspect(q); err != nil {
			t.Fatal(err)
		}
	}
	d, ok, err := ch.Get(KitchenPaidQueue, false)
	if err != nil || !ok {
		t.Fatalf("missing kitchen delivery: %v", err)
	}
	other, ok, err := ch.Get(OrderPaidQueue, false)
	if err != nil || !ok {
		t.Fatalf("missing order delivery: %v", err)
	}
	if err := other.Ack(false); err != nil {
		t.Fatal(err)
	}
	if err := HandleRetryToQueue(context.Background(), ch, &d, KitchenPaidQueue); err != nil {
		t.Fatal(err)
	}
	if err := d.Ack(false); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.QueueInspect(KitchenPaidQueue); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := ch.Get(OrderPaidQueue, true); err != nil || ok {
		t.Fatalf("retry leaked to successful order consumer: ok=%v err=%v", ok, err)
	}
	retry, ok, err := ch.Get(KitchenPaidQueue, true)
	if err != nil || !ok || string(retry.Body) != "paid" {
		t.Fatalf("retry missing: ok=%v err=%v", ok, err)
	}
	if retry.Headers[amqpRetryHeaderKey] == nil {
		t.Fatal("retry counter lost")
	}
}

func TestConfirmedPublisherRejectsUnroutableMessages(t *testing.T) {
	url := os.Getenv("GORDER_TEST_AMQP_URL")
	if url == "" {
		t.Fatal("GORDER_TEST_AMQP_URL required")
	}
	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	if err := DeclareTopology(ch); err != nil {
		t.Fatal(err)
	}
	p, err := NewConfirmedPublisher(ch)
	if err != nil {
		t.Fatal(err)
	}
	const queue = "test.confirmed.publisher"
	if _, err := ch.QueueDeclare(queue, true, false, false, false, nil); err != nil { t.Fatal(err) }
	defer ch.QueueDelete(queue, false, false, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.PublishWithContext(ctx, "", queue, false, true, amqp.Publishing{Body: []byte("test"), DeliveryMode: amqp.Persistent}); err != nil {
		t.Fatal(err)
	}
	if err := p.PublishWithContext(ctx, "", "missing-test-queue", false, true, amqp.Publishing{Body: []byte("test")}); err == nil || !strings.Contains(err.Error(), "unroutable") {
		t.Fatalf("expected unroutable error, got %v", err)
	}
}
