package relay

import (
	"encoding/json"
	"errors"

	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
)

// General base message implementation. Nostr uses JSON arrays so every message just goes through the
// standard json library as the last step before going out.
type EventMessage struct {
	Event *nostr.Event
}

func (m EventMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{EVENT_CODE, m.Event}) // NOTE: Here in case JSON encoding requires custom implementations
}

// Message sent to relay that terminates a subscription event started by REQ.
type CloseMessage struct {
	SubscriptionID string
}

func (m CloseMessage) MarshalJSON() ([]byte, error) {
	if m.SubscriptionID == "" {
		return nil, errors.New("Subscription ID cannot be empty")
	}

	return json.Marshal([]any{CLOSE_CODE, m.SubscriptionID})
}

// A REQ starts a subscription and provides filters which inform the relay on what events it should
// send back to the client.
type ReqMessage struct {
	SubscriptionID string
	Filters        []Filter
}

// TODO: Expand Filter to be more structured, concise and robust

type Filter map[string]any

func (m ReqMessage) MarshalJSON() ([]byte, error) {
	if m.SubscriptionID == "" {
		return nil, errors.New("Subscription ID cannot be empty")
	}

	if len(m.Filters) == 0 {
		return nil, errors.New("REQ must contain at least one filter")
	}

	msg := []any{REQ_CODE, m.SubscriptionID}
	for _, filter := range m.Filters {
		msg = append(msg, filter)
	}

	return json.Marshal(msg)
}
