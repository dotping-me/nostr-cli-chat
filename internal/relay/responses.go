package relay

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
)

// Code here (running out of juice on how to start these introductions) is responsible for parsing responses
// from the relays, AKA what the clients receive

type RelayMessage interface {
	Type() string
	Handler(raw []json.RawMessage) error // Basically will validate each field one by one
}

const (
	OK_CODE     = "OK"
	EVENT_CODE  = "EVENT"
	EOSE_CODE   = "EOSE"
	NOTICE_CODE = "NOTICE"
	CLOSE_CODE  = "CLOSE"
)

// Response structure to success/acknowledgement messages (i.e. Status 200).
type OKMessage struct {
	EventID  string
	Accepted bool
	Message  string
}

func (m OKMessage) Type() string {
	return OK_CODE
}

func (m *OKMessage) Handler(raw []json.RawMessage) error {
	if len(raw) != 4 {
		return fmt.Errorf("Invalid %s message length", OK_CODE)
	}

	if err := json.Unmarshal(raw[1], &m.EventID); err != nil {
		return err
	}

	if err := json.Unmarshal(raw[2], &m.Accepted); err != nil {
		return err
	}

	if err := json.Unmarshal(raw[3], &m.Message); err != nil {
		return err
	}

	return nil
}

// Response structure for Event messages, typically started by sending a REQ message to the relay.
type IncomingEventMessage struct {
	SubscriptionID string
	Event          nostr.Event
}

func (m IncomingEventMessage) Type() string {
	return EVENT_CODE
}

func (m *IncomingEventMessage) Handler(raw []json.RawMessage) error {
	if len(raw) != 3 {
		return fmt.Errorf("Invalid %s message length", EVENT_CODE)
	}

	if err := json.Unmarshal(raw[1], &m.SubscriptionID); err != nil {
		return err
	}

	if err := json.Unmarshal(raw[2], &m.Event); err != nil {
		return err
	}

	return nil
}

// Response structure for End-Of-Store-Event messages meaning that there are no more messages for the subscribed
// events currently.
type EOSEMessage struct {
	SubscriptionID string
}

func (m EOSEMessage) Type() string {
	return EOSE_CODE
}

func (m *EOSEMessage) Handler(raw []json.RawMessage) error {
	if len(raw) != 2 {
		return fmt.Errorf("Invalid %s message length", EOSE_CODE)
	}

	if err := json.Unmarshal(raw[1], &m.SubscriptionID); err != nil {
		return err
	}

	return nil
}

// Response structure for notifications sent by the relay.
type NoticeMessage struct {
	Message string
}

func (m NoticeMessage) Type() string {
	return NOTICE_CODE
}

func (m *NoticeMessage) Handler(raw []json.RawMessage) error {
	if len(raw) != 2 {
		return fmt.Errorf("Invalid %s message length", NOTICE_CODE)
	}

	if err := json.Unmarshal(raw[1], &m.Message); err != nil {
		return err
	}

	return nil
}

// Parses the received JSON array from the relay to obtain information
func ParseIncomingMessageFromRelay(data []byte) (RelayMessage, error) {

	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("Invalid message from relay: %w", err)
	}

	if len(raw) == 0 {
		return nil, errors.New("Empty relay message received")
	}

	var msgType string
	if err := json.Unmarshal(raw[0], &msgType); err != nil {
		return nil, errors.New("Relay message type must be a string")
	}

	var msg RelayMessage
	switch msgType {
	case OK_CODE:
		msg = &OKMessage{}
	case EVENT_CODE:
		msg = &IncomingEventMessage{}
	case EOSE_CODE:
		msg = &EOSEMessage{}
	case NOTICE_CODE:
		msg = &NoticeMessage{}
	default:
		return nil, fmt.Errorf("Unsupported relay message type: %s", msgType)
	}

	if err := msg.Handler(raw); err != nil {
		return nil, err
	}

	return msg, nil
}
