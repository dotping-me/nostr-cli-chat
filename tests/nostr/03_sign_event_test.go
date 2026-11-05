package tests

import (
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
)

// Testing proper event signatures as well as valid keypairs
func TestEventSign(t *testing.T) {
	privKey, err := btcec.NewPrivateKey() // Generates a keypair
	if err != nil {
		t.Fatal(err)
	}

	pubKey := schnorr.SerializePubKey(privKey.PubKey())
	event := nostr.Event{
		PubKey:    hex.EncodeToString(pubKey),
		CreatedAt: 1730000000,
		Kind:      1,
		Tags:      [][]string{},
		Content:   "Hello Nostr!",
	}

	// Manually signs the event using the private key
	if err := event.Sign(privKey); err != nil {
		t.Fatal(err)
	}

	// Validate ID and signature encoding
	if len(event.ID) != 64 {
		t.Fatalf("Expected 64-character event ID, got %d", len(event.ID))
	}

	if len(event.Sig) != 128 {
		t.Fatalf("Expected 128-character signature, got %d", len(event.Sig))
	}

	// Decode the ID and signature into raw bytes and parses them
	idBytes, err := hex.DecodeString(event.ID)
	if err != nil {
		t.Fatal(err)
	}

	sigBytes, err := hex.DecodeString(event.Sig)
	if err != nil {
		t.Fatal(err)
	}

	sig, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		t.Fatal(err)
	}

	pubKeyParsed, err := schnorr.ParsePubKey(pubKey)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the BIP-340 signature
	if !sig.Verify(idBytes, pubKeyParsed) {
		t.Fatal("Signature verification failed")
	}

	// Verify entire event integrity and authenticity (This is just extra)
	if err := event.Verify(); err != nil {
		t.Fatalf("Expected valid event, got error: %v", err)
	}
}

// Testing if event verification implementation catches tampered content. Note that this test passes if
// implementation correctly identifies tamper and returns an error.
func TestEventVerifyTamperedPayload(t *testing.T) {

	// Valid keypairs are generated once
	// These will be used as the valid keys for the payload

	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}

	pubKey := hex.EncodeToString(schnorr.SerializePubKey(privKey.PubKey()))

	// These keys (generated right after this comment) will be used to tamper with the payload
	otherPrivKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}

	otherPubKey := hex.EncodeToString(schnorr.SerializePubKey(otherPrivKey.PubKey()))

	tests := []struct {
		field  string
		mutate func(*nostr.Event)
	}{
		{
			field: "content",
			mutate: func(e *nostr.Event) {
				e.Content = "Tampered content"
			},
		},
		{
			field: "kind",
			mutate: func(e *nostr.Event) {
				e.Kind = 2
			},
		},
		{
			field: "created_at",
			mutate: func(e *nostr.Event) {
				e.CreatedAt++
			},
		},
		{
			field: "pubkey",
			mutate: func(e *nostr.Event) {
				e.PubKey = otherPubKey
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {

			// 1. Creates and signs the event legimatedly
			event := &nostr.Event{
				PubKey:    pubKey,
				CreatedAt: 1674322515,
				Kind:      1,
				Tags:      [][]string{},
				Content:   "Hello Nostr!",
			}

			if err := event.Sign(privKey); err != nil {
				t.Fatal(err)
			}

			// 2. Tamper with specific field in payload
			tt.mutate(event)
			if err := event.Verify(); err == nil {
				t.Fatalf("Expected verification to fail for tampered %s", tt.field)
			}
		})
	}
}
