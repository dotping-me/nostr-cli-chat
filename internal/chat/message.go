package chat

import (
	"errors"

	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
)

type Message struct {
	ID        string
	Sender    string
	Recipient string
	Content   string
	CreatedAt int64
}

// Converts a NIP-01 event into a chat message and only accepts kind-1 events addressed to the expected participant.
func MessageFromEvent(event *nostr.Event, localPubKey, remotePubKey string) (*Message, error) {
	if event == nil {
		return nil, errors.New("event cannot be nil")
	}

	if err := event.Verify(); err != nil {
		return nil, err
	}

	if event.Kind != 1 {
		return nil, errors.New("event is not a kind-1 text note")
	}

	var expectedRecipient string

	switch event.PubKey {
	case localPubKey:
		expectedRecipient = remotePubKey

	case remotePubKey:
		expectedRecipient = localPubKey

	default:
		return nil, errors.New("event is not from a conversation participant")
	}

	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "p" && tag[1] == expectedRecipient {
			return &Message{
				ID:        event.ID,
				Sender:    event.PubKey,
				Recipient: expectedRecipient,
				Content:   event.Content,
				CreatedAt: event.CreatedAt,
			}, nil
		}
	}

	return nil, errors.New("event does not belong to this conversation")
}
