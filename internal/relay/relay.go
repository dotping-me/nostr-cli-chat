package relay

import (
	"context"
	"errors"
	"fmt"

	"github.com/coder/websocket"
	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
)

// Relevant implementation regarding relays; connections, messaging and so on. Right now, I am using an
// open relay instead of implementing one

type Relay struct {
	URL  string
	conn *websocket.Conn
}

const (
	OK_CODE     = "OK"
	EVENT_CODE  = "EVENT"
	EOSE_CODE   = "EOSE"
	NOTICE_CODE = "NOTICE"
	CLOSE_CODE  = "CLOSE"
	REQ_CODE    = "REQ"
)

// Tries to establish a websocket connection to a relay.
func Connect(ctx context.Context, url string) (*Relay, error) {
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to relay (%s): %w", url, err)
	}

	return &Relay{URL: url, conn: conn}, nil
}

// Sends a message through the relay.
func (r *Relay) Send(ctx context.Context, msg []byte) error {
	if err := r.conn.Write(ctx, websocket.MessageText, msg); err != nil {
		return fmt.Errorf("Failed to send message: %w", err)
	}

	return nil
}

// Sends an event from predefined types to relay. Message types implementations are found in /messages/*.
func (r *Relay) SendEvent(ctx context.Context, event *nostr.Event) error {
	msg, err := EventMessage{Event: event}.MarshalJSON() // NOTE: Nostr Event payload creation is handled elsewhere
	if err != nil {
		return fmt.Errorf("Failed to encode EVENT: %w", err)
	}

	return r.Send(ctx, msg)
}

// Sends an Event to the relay and waits (blocking) for an acknowledgement indicating that the event has been
// successfully published on the network and can be received with appropriate subscriptions.
func (r *Relay) Publish(ctx context.Context, event *nostr.Event) (*OKMessage, error) {
	if event == nil {
		return nil, errors.New("Event cannot be nil")
	}

	if err := event.Verify(); err != nil {
		return nil, fmt.Errorf("Failed to publish Event: %w", err)
	}

	// Serialize the outgoing event
	// TODO: Implement NIP-04 for encrypted DMs

	data, err := EventMessage{Event: event}.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("Failed to encode Event to publish: %w", err)
	}

	// Sends the event and then waits for the acknowledgement response from the relay. Note that currently,
	// waiting for relay acknowledgement is blocking

	if err := r.Send(ctx, data); err != nil {
		return nil, fmt.Errorf("Failed to send Event: %w", err)
	}

	for {
		data, err := r.Read(ctx)
		if err != nil {
			return nil, fmt.Errorf("Failed to read response when waiting for ACK: %w", err)
		}

		msg, err := ParseIncomingMessageFromRelay(data)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse response from relay: %w", err)
		}

		switch m := msg.(type) {
		case *OKMessage:
			if m.EventID != event.ID { // Ignore acknowledgements for unrelated events
				continue
			}

			return m, nil

		case *NoticeMessage:
			continue // Purely informational
		}
	}
}

// Sends a REQ message to the relay, subscribing to receive updates according to specifications. Note that this does
// not send an event to the relay and as per NIP-01 docs: ["REQ", <subscription_id>, <filters1>, <filters2>, ...].
func (r *Relay) Subscribe(ctx context.Context, req ReqMessage) error {
	data, err := req.MarshalJSON()
	if err != nil {
		return fmt.Errorf("Failed to encode %s: %w", REQ_CODE, err)
	}

	if err := r.Send(ctx, data); err != nil {
		return fmt.Errorf("Failed to send %s: %w", REQ_CODE, err)
	}

	return nil
}

// Reads a message from the relay.
func (r *Relay) Read(ctx context.Context) ([]byte, error) {
	_, msg, err := r.conn.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to read message: %w", err)
	}

	return msg, nil
}

// Receives and parses events received from the relay synchronously.
func (r *Relay) ReceiveEventSync(ctx context.Context) (RelayResponse, error) {
	data, err := r.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to read relay response: %w", err)
	}

	msg, err := ParseIncomingMessageFromRelay(data)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse relay response: %w", err)
	}

	return msg, nil
}

// Sends a CLOSE message to the relay indicating a termination of a given subscription. Note that this does not
// terminate the websocket connection however.
func (r *Relay) CloseSubscription(ctx context.Context, subscriptionID string) error {
	msg, err := CloseMessage{SubscriptionID: subscriptionID}.MarshalJSON()
	if err != nil {
		return fmt.Errorf("Failed to encode %s: %w", CLOSE_CODE, err)
	}

	if err := r.Send(ctx, msg); err != nil {
		return fmt.Errorf("Failed to send %s: %w", CLOSE_CODE, err)
	}

	return nil
}

// Terminates connection between client and relay.
func (r *Relay) Close() error {
	err := r.conn.Close(websocket.StatusNormalClosure, websocket.StatusNormalClosure.String())
	if err != nil {
		return fmt.Errorf("Failed to close relay connection: %w", err)
	}

	return nil
}
