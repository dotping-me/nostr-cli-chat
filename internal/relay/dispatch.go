package relay

import (
	"log/slog"
)

// The dispatcher is the core of a Relay: a single goroutine reads from the websocket and routes inbound traffic to
// the channels registered for pending publications and subscriptions. The sync and async APIs only consume those
// channels, which guarantees there is exactly one reader per connection.

// Dispatcher lifecycle loop, i.e. routing inbound traffic to relevant channels.
func (r *Relay) readLoop() {
	defer func() {
		r.cancel()
		r.cleanup()
		close(r.done)
		slog.Info("Relay dispatcher stopped", "url", r.URL)
	}()

	for {
		_, data, err := r.conn.Read(r.ctx)
		if err != nil {
			if r.ctx.Err() == nil {
				slog.Error("Relay read failed", "url", r.URL, "error", err)
			}

			return
		}

		msg, err := ParseIncomingMessageFromRelay(data)
		if err != nil {
			slog.Error("Failed to parse relay message", "error", err)
			continue
		}

		r.dispatch(msg)
	}
}

// Internal dispatcher to route several different responses from relay in a non-blocking fashion using a single
// websocket connection.
func (r *Relay) dispatch(msg RelayResponse) {
	switch m := msg.(type) {
	case *OKMessage:
		r.dispatchOK(m)

	case *IncomingEventMessage:
		r.routeSubscription(m.SubscriptionID, m)

	case *EOSEMessage:
		r.routeSubscription(m.SubscriptionID, m)
	}
}

// Triggered after an event has been published to a relay and an ACK comes back. Removes the pending entry and
// delivers the message to the channel registered when the event was published.
func (r *Relay) dispatchOK(msg *OKMessage) {
	r.mu.Lock()

	ch := r.pending[msg.EventID]
	delete(r.pending, msg.EventID)

	r.mu.Unlock()

	if ch != nil {
		ch <- msg
	}
}

// Routes a subscription message to its channel in arrival order. Called only from the dispatcher goroutine; cleanup
// runs after this loop has stopped, so a channel is never closed while a send is in flight.
func (r *Relay) routeSubscription(subscriptionID string, msg RelayResponse) {
	r.mu.Lock()
	sub := r.subscriptions[subscriptionID]

	r.mu.Unlock()

	if sub == nil {
		return
	}

	select {
	case sub.Messages <- msg:
	case <-r.ctx.Done():
	}
}

// Closing channels once the dispatcher has stopped. No other goroutine sends on these channels at this point.
func (r *Relay) cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Notify pending publishers that channels are closed
	for _, ch := range r.pending {
		close(ch)
	}

	// Notify subscription consumers that channels are closed
	for _, sub := range r.subscriptions {
		close(sub.Messages)
	}

	clear(r.pending)
	clear(r.subscriptions)
	r.syncSub = nil
}
