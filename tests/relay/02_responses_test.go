package relay_test

import (
	"testing"

	"github.com/dotping-me/nostr-cli-chat/internal/relay"
)

// Testing correctly parsing received JSON payloads from relay to client.
func TestParseMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "OK accepted",
			input:    `["OK","abc",true,""]`,
			expected: "OK",
		},
		{
			name:     "OK rejected",
			input:    `["OK","abc",false,"invalid signature"]`,
			expected: "OK",
		},
		{
			name:     "EVENT",
			input:    `["EVENT","chat",{"id":"abc","pubkey":"def","created_at":123,"kind":1,"tags":[],"content":"Hello","sig":"ghi"}]`,
			expected: "EVENT",
		},
		{
			name:     "EOSE",
			input:    `["EOSE","chat"]`,
			expected: "EOSE",
		},
		{
			name:     "NOTICE",
			input:    `["NOTICE","rate limited"]`,
			expected: "NOTICE",
		},
		{
			name:    "empty message",
			input:   `[]`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   `invalid`,
			wantErr: true,
		},
		{
			name:    "unsupported type",
			input:   `["UNKNOWN"]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := relay.ParseIncomingMessageFromRelay([]byte(tt.input))

			if tt.wantErr {
				if err == nil {
					t.Fatal("Expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if msg.Type() != tt.expected {
				t.Errorf("Expected type %s, got %s", tt.expected, msg.Type())
			}
		})
	}
}
