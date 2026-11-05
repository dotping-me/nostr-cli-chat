package relay_test

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
	"github.com/dotping-me/nostr-cli-chat/internal/relay"
)

func generateKeypair(t *testing.T) (*btcec.PrivateKey, string) {
	t.Helper()
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}

	return privKey, hex.EncodeToString(schnorr.SerializePubKey(privKey.PubKey()))
}

// Tests the whole Event to Relay and back workflow.
func TestPublishEventToRelay(t *testing.T) {

	// 0. Generate valid keypair
	privKey, pubKey := generateKeypair(t)

	// 1. Establish connection to relay
	// NOTE: It may be bad practice to have a real connection here as a test and testing multiple times would mean
	// 		 multiple TCP connections so maybe I should implement something like testing.Short() and t.Skip()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r, err := relay.ConnectSync(ctx, "wss://relay.primal.net")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	defer r.Close()

	if r.URL != "wss://relay.primal.net" {
		t.Errorf("Unexpected relay URL: %s", r.URL)
	}

	// 2. Register and sign the event payload
	event := nostr.Event{
		PubKey:    pubKey,
		CreatedAt: time.Now().Unix(),
		Kind:      1,
		Tags:      [][]string{},
		Content:   "Hello World!",
	}

	if err := event.Sign(privKey); err != nil {
		t.Fatal(err)
	}

	ack, err := r.PublishSync(ctx, &event)
	if err != nil {
		t.Fatal(err)
	}

	if ack.Accepted {
		t.Logf("Event accepted: %s", ack.EventID)
	} else {
		t.Logf("Event rejected: %s", ack.Message)
	}
}
