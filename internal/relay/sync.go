package relay

import (
	"context"
	"errors"
	"fmt"

	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
)

// Synchronous API blocking wrappers over the dispatcher-backed channels used by async.go

// Connects to a relay using the dispatcher for inbound traffic.
func ConnectSync(ctx context.Context, url string) (*Relay, error) {
	return connect(ctx, url)
}

// Sends an Event to the relay and waits (blocking) for an acknowledgement indicating that the event has been
// successfully published on the network and can be received with appropriate subscriptions.
func (r *Relay) PublishSync(ctx context.Context, event *nostr.Event) (*OKMessage, error) {
	ackCh, err := r.PublishAsync(ctx, event)
	if err != nil {
		return nil, err
	}

	select {
	case ack, ok := <-ackCh:
		if !ok {
			return nil, errors.New("Relay disconnected before ACK")
		}

		return ack, nil

	case <-ctx.Done():
		return nil, fmt.Errorf("Failed to wait for ACK: %w", ctx.Err())
	}
}

// Sends a REQ message and remembers the subscription for ReceiveEventSync. Only one synchronous subscription can
// be active per connection because ReceiveEventSync reads from a single ordered stream.
func (r *Relay) SubscribeSync(ctx context.Context, req ReqMessage) error {
	r.mu.Lock()
	active := r.syncSub != nil
	r.mu.Unlock()

	if active {
		return errors.New("Synchronous API supports one active subscription per connection")
	}

	sub, err := r.SubscribeAsync(ctx, req)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.syncSub = sub
	r.mu.Unlock()

	return nil
}

// Returns the next message routed to the subscription created by SubscribeSync.
func (r *Relay) ReceiveEventSync(ctx context.Context) (RelayResponse, error) {
	r.mu.Lock()
	sub := r.syncSub
	r.mu.Unlock()

	if sub == nil {
		return nil, errors.New("No active subscription; call SubscribeSync first")
	}

	select {
	case msg, ok := <-sub.Messages:
		if !ok {
			return nil, errors.New("Relay disconnected")
		}

		return msg, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
