package relay_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
	"github.com/dotping-me/nostr-cli-chat/internal/relay"
)

// Attemps to connect to an open relay to test connectivity.
func TestRelayConnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r, err := relay.Connect(ctx, "wss://relay.primal.net")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	defer r.Close()

	if r.URL != "wss://relay.primal.net" {
		t.Errorf("Unexpected relay URL: %s", r.URL)
	}
}

// Testing if event messages are correctly encoded into JSON arrays before sending to relays. Note that the proper
// implementation of this is by integration with Nostr Event implementations in /internal/nostr/*.
func TestEventMessageMarshal(t *testing.T) {
	msg := relay.EventMessage{
		Event: &nostr.Event{
			ID:        "abc",
			PubKey:    "def",
			CreatedAt: 123,
			Kind:      1,
			Tags:      [][]string{},
			Content:   "Hello",
			Sig:       "ghi",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	expected := `["EVENT",{"id":"abc","pubkey":"def","created_at":123,"kind":1,"tags":[],"content":"Hello","sig":"ghi"}]`
	if string(data) != expected {
		t.Fatalf("Failed marshal!\nExpected\t%s'\nGot\t\t%s", expected, data)
	}
}
