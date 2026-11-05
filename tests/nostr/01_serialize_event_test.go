package nostr_test

import (
	"bytes"
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
