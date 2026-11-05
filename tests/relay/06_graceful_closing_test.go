package relay_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/dotping-me/nostr-cli-chat/internal/relay"
)

func TestCloseIsIdempotent(t *testing.T) {

	// Uses a local HTTP server that provides Websocket connections instead of establishing a connection to a
	// relay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
	}))
	defer server.Close()

	r, err := relay.ConnectSync(context.Background(), "ws"+server.URL[4:])
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAsyncDisconnectClosesSubscriptionChannels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}

		// Waits for the REQ so subscribing succeeds before the simulated disconnect.
		if _, _, err := conn.Read(r.Context()); err != nil {
			return
		}

		conn.Close(websocket.StatusInternalError, "Early disconnect sim")
	}))
	defer server.Close()

	_, pubKey := generateKeypair(t)

	r, err := relay.ConnectAsync(context.Background(), "ws"+server.URL[4:])
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	req := relay.ReqMessage{
		SubscriptionID: "test-sub",
		Filters: []relay.Filter{
			{
				"authors": []string{pubKey},
				"kinds":   []int{1},
			},
		},
	}

	sub, err := r.SubscribeAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	select {
	case _, ok := <-sub.Messages:
		if ok {
			t.Fatal("Expected Messages channel to be closed")
		}

	case <-time.After(time.Second):
		t.Fatal("Messages channel was not closed")
	}
}
