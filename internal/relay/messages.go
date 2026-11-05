package relay

import (
	"encoding/json"

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
	return json.Marshal([]any{CLOSE_CODE, m.SubscriptionID})
}

// A REQ starts a subscription and provides filters which inform the relay on what events it should
// send back to the client.
type ReqMessage struct {
	SubscriptionID string
	Filters        []Filter
}

type Filter map[string]any

// Helper function to convert filters from the Filter struct to Any type because this one is recognised by the
// json standard library.
func filtersToAny(filters []Filter) []any {
	result := make([]any, len(filters))
	for i, filter := range filters {
		result[i] = filter
	}

	return result
}

func (m ReqMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(append([]any{"REQ", m.SubscriptionID}, filtersToAny(m.Filters)...))
}
