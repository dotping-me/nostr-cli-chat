package nostr_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
)

// Testing method implementation for ID computation against manually/independently hashing the ID.
func TestEventComputeID(t *testing.T) {
	event := nostr.Event{
		PubKey:    generatePublicKey(t),
		CreatedAt: 1730000000,
		Kind:      1,
		Tags: [][]string{
			{"p", "abcdef123456"},
			{"t", "golang"},
		},
		Content: "Hello Nostr!",
	}

	got, err := event.ComputeID() // NOTE: Internally handles serialization
	if err != nil {
		t.Fatal(err)
	}

	// Independently calculate the expected ID using SHA-256
	serialized, err := event.Serialize()
	if err != nil {
		t.Fatal(err)
	}

	hash := sha256.Sum256(serialized)
	want := hex.EncodeToString(hash[:])

	if got != want {
		t.Fatalf("Event ID mismatch:\nGot:  %s\nWant: %s", got, want)
	}

	if len(got) != 64 {
		t.Fatalf("Expected 64-character event ID, got %d", len(got))
	}
}

// Testing ID computation against a known valid ID from source https://armstrys.github.io/nostrfastr/nostr_core.html.
func TestEventComputeIDKnownEvent(t *testing.T) {
	event := nostr.Event{
		PubKey:    "91feaf1b03c1dc0ed61163f77b4cf4d1821cce392983d4c68d82207a773f12ee",
		CreatedAt: 1674322515,
		Kind:      1,
		Tags:      [][]string{},
		Content:   "this is a test",
	}

	const expectedID = "589ad714c2a36929abd4a817a2177c594d31e5c373948af9c22890f5590c17f6"

	got, err := event.ComputeID()
	if err != nil {
		t.Fatal(err)
	}

	if got != expectedID {
		t.Fatalf("Event ID mismatch:\nGot:  %s\nWant: %s", got, expectedID)
	}
}
