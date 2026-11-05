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

// Reads a message from the relay.
func (r *Relay) Read(ctx context.Context) ([]byte, error) {
	_, msg, err := r.conn.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to read message: %w", err)
	}

	return msg, nil
}

// Terminates connection between client and relay.
func (r *Relay) Close() error {
	err := r.conn.Close(websocket.StatusNormalClosure, websocket.StatusNormalClosure.String())
	if err != nil {
		return fmt.Errorf("Failed to close relay connection: %w", err)
	}

	return nil
}
