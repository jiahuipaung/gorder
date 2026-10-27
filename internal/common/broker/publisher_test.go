package broker

import (
	"context"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

type controlledChannel struct {
	publishes int
	closes    int
}

func (c *controlledChannel) PublishWithContext(context.Context, string, string, bool, bool, amqp.Publishing) error {
	c.publishes++
	return nil
}

func (c *controlledChannel) Close() error { c.closes++; return nil }

func TestTimedOutConfirmationPoisonsPublisherBeforeNextMessage(t *testing.T) {
	channel := &controlledChannel{}
	acks := make(chan amqp.Confirmation, 1)
	p := &ConfirmedPublisher{ch: channel, acks: acks, returns: make(chan amqp.Return)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.PublishWithContext(ctx, "", "order.create", true, false, amqp.Publishing{}); err == nil {
		t.Fatal("timed-out publication was accepted")
	}
	if channel.closes != 1 {
		t.Fatalf("uncertain channel must close once; closes=%d", channel.closes)
	}
	acks <- amqp.Confirmation{Ack: true, DeliveryTag: 1} // late ACK for the first publish
	if err := p.PublishWithContext(context.Background(), "", "order.create", true, false, amqp.Publishing{}); err == nil {
		t.Fatal("publisher reused a channel with an uncertain confirmation")
	}
	if channel.publishes != 1 {
		t.Fatalf("second publication reached poisoned channel; publishes=%d", channel.publishes)
	}
}
