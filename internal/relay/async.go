package relay

import (
	"context"
	"errors"
	"fmt"

	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
)

// Asynchronous API sends return immediately and inbound messages are consumed from channels fed by the dispatcher

// Connects to a relay using the dispatcher for inbound traffic.
func ConnectAsync(ctx context.Context, url string) (*Relay, error) {
	return connect(ctx, url)
}

// Sends an Event to the relay and returns a channel through which the ACK will be received later on.
func (r *Relay) PublishAsync(ctx context.Context, event *nostr.Event) (<-chan *OKMessage, error) {
	if event == nil {
		return nil, errors.New("Event cannot be nil")
	}

	if err := event.Verify(); err != nil {
		return nil, fmt.Errorf("Failed to publish Event: %w", err)
	}

	data, err := EventMessage{Event: event}.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("Failed to encode Event to publish: %w", err)
	}

	// Registers a dedicated channel for this pending ACK
	ackCh := make(chan *OKMessage, 1)
	r.mu.Lock()

	if _, exists := r.pending[event.ID]; exists {
		r.mu.Unlock()
		return nil, fmt.Errorf("Event %s already pending", event.ID)
	}

	r.pending[event.ID] = ackCh
	r.mu.Unlock()

	// Sends over the event and cleans up channels in case of failure
	if err := r.Send(ctx, data); err != nil {
		r.mu.Lock()
		delete(r.pending, event.ID)
		r.mu.Unlock()

		return nil, fmt.Errorf("Failed to send Event: %w", err)
	}

	return ackCh, nil
}

// Sends a REQ message to the relay and returns a subscription whose Messages channel receives EVENT and EOSE
// messages. The subscription is registered before the REQ is sent so a fast EOSE cannot race registration.
func (r *Relay) SubscribeAsync(ctx context.Context, req ReqMessage) (*Subscription, error) {
	data, err := req.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("Failed to encode %s: %w", REQ_CODE, err)
	}

	sub := &Subscription{ // Registers the REQ with sized buffers
		ID:       req.SubscriptionID,
		Messages: make(chan RelayResponse, 16),
	}

	r.mu.Lock()
	if _, exists := r.subscriptions[sub.ID]; exists {
		r.mu.Unlock()
		return nil, fmt.Errorf("Subscription %q already exists", sub.ID)
	}

	r.subscriptions[sub.ID] = sub
	r.mu.Unlock()

	// Proceeds with normal transmission flow
	if err := r.Send(ctx, data); err != nil {

		// Cleanup in case of transmission failure
		r.mu.Lock()
		delete(r.subscriptions, sub.ID)
		r.mu.Unlock()

		return nil, fmt.Errorf("Failed to send %s: %w", REQ_CODE, err)
	}

	return sub, nil
}
