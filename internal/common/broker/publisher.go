package broker

import (
	"context"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ConfirmedPublisher serializes publishes on one channel and waits for a broker
// confirm. A returned mandatory message is never reported as accepted.
type ConfirmedPublisher struct {
	ch       publishChannel
	mu       sync.Mutex
	acks     <-chan amqp.Confirmation
	returns  <-chan amqp.Return
	poisoned bool
}

type publishChannel interface {
	PublishWithContext(context.Context, string, string, bool, bool, amqp.Publishing) error
	Close() error
}

func NewConfirmedPublisher(ch *amqp.Channel) (*ConfirmedPublisher, error) {
	if ch == nil {
		return nil, fmt.Errorf("broker channel is nil")
	}
	if err := ch.Confirm(false); err != nil {
		return nil, err
	}
	return &ConfirmedPublisher{
		ch:      ch,
		acks:    ch.NotifyPublish(make(chan amqp.Confirmation, 1)),
		returns: ch.NotifyReturn(make(chan amqp.Return, 1)),
	}, nil
}

var publishers = struct {
	sync.Mutex
	byChannel map[*amqp.Channel]*ConfirmedPublisher
}{byChannel: make(map[*amqp.Channel]*ConfirmedPublisher)}

func ConfirmedPublisherFor(ch *amqp.Channel) (*ConfirmedPublisher, error) {
	publishers.Lock()
	defer publishers.Unlock()
	if p := publishers.byChannel[ch]; p != nil {
		return p, nil
	}
	p, err := NewConfirmedPublisher(ch)
	if err != nil {
		return nil, err
	}
	publishers.byChannel[ch] = p
	return p, nil
}

func (p *ConfirmedPublisher) PublishWithContext(ctx context.Context, exchange, key string, _ bool, _ bool, msg amqp.Publishing) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.poisoned {
		return fmt.Errorf("publisher channel retired after uncertain confirmation")
	}
	if err := p.ch.PublishWithContext(ctx, exchange, key, true, false, msg); err != nil {
		p.retire()
		return err
	}
	var returned *amqp.Return
	for {
		select {
		case <-ctx.Done():
			p.retire()
			return ctx.Err()
		case ret, ok := <-p.returns:
			if !ok {
				p.retire()
				return fmt.Errorf("broker channel closed before return")
			}
			returned = &ret
		case ack, ok := <-p.acks:
			if !ok {
				p.retire()
				return fmt.Errorf("broker channel closed before confirm")
			}
			if !ack.Ack {
				return fmt.Errorf("broker rejected message")
			}
			if returned == nil {
				select {
				case ret := <-p.returns:
					returned = &ret
				default:
				}
			}
			if returned != nil {
				return fmt.Errorf("unroutable message: %d %s", returned.ReplyCode, returned.ReplyText)
			}
			return nil
		}
	}
}

// A timed-out publish may still receive a late ACK. Retiring its channel
// prevents that ACK from being mistaken for a later message's confirmation.
func (p *ConfirmedPublisher) retire() {
	if p.poisoned {
		return
	}
	p.poisoned = true
	_ = p.ch.Close()
}
