package tests

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
)

// Helper function that generates a valid keypair and returns the 32-byte hex public key.
func generatePublicKey(t *testing.T) string {
	t.Helper()
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}

	return hex.EncodeToString(schnorr.SerializePubKey(privKey.PubKey()))
}

// Testing content serialization from NIP-01 event payload with custom character escape implementation.
func TestEscapeAndSerializeContent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", `"hello"`},
		{"hello\nworld", `"hello\nworld"`},
		{`say "hello"`, `"say \"hello\""`},
		{`C:\Users`, `"C:\\Users"`},
		{"hello\rworld", `"hello\rworld"`},
		{"hello\tworld", `"hello\tworld"`},
		{"hello\bworld", `"hello\bworld"`},
		{"hello\fworld", `"hello\fworld"`},
		{"<hello & goodbye>", `"<hello & goodbye>"`},
		{"café 🌍", `"café 🌍"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := nostr.EscapeAndSerializeContent(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != tt.want {
				t.Fatalf("Got %q, Want %q", got, tt.want)
			}
		})
	}
}

// Testing whole NIP-01 event serialization process.
func TestEventSerialize(t *testing.T) {
	pubKey := generatePublicKey(t)
	event := nostr.Event{
		PubKey:    pubKey,
		CreatedAt: 1730000000,
		Kind:      1,
		Tags: [][]string{
			{"p", "abcdef123456"},
			{"t", "golang"},
		},
		Content: "Hello Nostr!",
	}

	got, err := event.Serialize()
	if err != nil {
		t.Fatal(err)
	}

	want := `[0,"` + pubKey + `",1730000000,1,[["p","abcdef123456"],["t","golang"]],"Hello Nostr!"]`
	if string(got) != want {
		t.Fatalf("Serialization mismatch:\nGot:  %s\nWant: %s", got, want)
	}
}

// Comparing custom event serialization implementation versus use of standard json library from marshalling. In some
// cases, both implementations can return the same correct output so this is more a question of consistency and
// reliability.
func TestEventSerializeAgainstJSON(t *testing.T) {
	tests := []struct {
		name    string
		content string
		match   bool
	}{
		{
			name:    "ordinary content",
			content: "Hello Nostr!",
			match:   true,
		},
		{
			name:    "HTML characters",
			content: "<hello & goodbye>",
			match:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// 1. Using custom implementation to serialize event payload
			event := nostr.Event{
				PubKey:    generatePublicKey(t),
				CreatedAt: 1730000000,
				Kind:      1,
				Tags: [][]string{
					{"p", "abcdef123456"},
					{"t", "golang"},
				},
				Content: tt.content,
			}

			got, err := event.Serialize()
			if err != nil {
				t.Fatal(err)
			}

			// 2. Using standard json library to serialize event payload
			payload := []any{0, event.PubKey, event.CreatedAt, event.Kind, event.Tags, event.Content}
			want, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}

			// 3. Compares the outputs from both implementations
			matches := bytes.Equal(got, want)
			if matches != tt.match {
				t.Fatalf("Unexpected serialization comparison:\nCustom:\t\t%s\nStandard:\t%s\nExpected match: %t\nActual match:\t%t",
					got, want, tt.match, matches)
			}
		})
	}
}

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
			field: "public_key",
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
