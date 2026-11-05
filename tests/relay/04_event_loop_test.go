package relay_test

import (
	"context"
	"testing"
	"time"

	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
	"github.com/dotping-me/nostr-cli-chat/internal/relay"
)

// Tests sending a REQ message to the relay. Note that this tests only the outgoing part.
func TestSubscribeToRelay(t *testing.T) {

	// 0. Generate valid keypair
	_, pubKey := generateKeypair(t)

	// 1. Establish connection to relay
	// NOTE: It may be bad practice to have a real connection here as a test and testing multiple times would mean
	// 		 multiple TCP connections so maybe I should implement something like testing.Short() and t.Skip()

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

	// 2. Send a REQ message
	req := relay.ReqMessage{
		SubscriptionID: "test-sub",
		Filters: []relay.Filter{
			{
				"authors": []string{pubKey},
				"kinds":   []int{1},
			},
		},
	}

	if err := r.Subscribe(ctx, req); err != nil {
		t.Fatal(err)
	}

	t.Log("Subscription sent successfully")
}

// Tests a full send-and-receive flow from client to relay. Client first publishes an event, then subscribes to receive
// that same event and handles responses from relay. This is just an integration of previous tests.
func TestPublishAndReceiveEvent(t *testing.T) {

	// 0. Generate valid keypair
	privKey, pubKey := generateKeypair(t)

	// 1. Establish connection to relay
	// NOTE: It may be bad practice to have a real connection here as a test and testing multiple times would mean
	// 		 multiple TCP connections so maybe I should implement something like testing.Short() and t.Skip()

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

	ack, err := r.Publish(ctx, &event)
	if err != nil {
		t.Fatal(err)
	}

	if ack.Accepted {
		t.Logf("Event accepted: %s", ack.EventID)
	} else {
		t.Fatalf("Event rejected: %s", ack.Message)
	}

	// 3. Sends REQ message to subscribe to receive that published event
	req := relay.ReqMessage{
		SubscriptionID: "test-sub",
		Filters: []relay.Filter{
			{
				"authors": []string{pubKey},
				"kinds":   []int{1},
			},
		},
	}

	if err := r.Subscribe(ctx, req); err != nil {
		t.Fatal(err)
	}

	t.Log("Subscription sent successfully")

	// 4. Parse responses from relay
	for {
		msg, err := r.ReceiveEventSync(ctx)
		if err != nil {
			t.Fatal(err)
		}

		switch m := msg.(type) {
		case *relay.IncomingEventMessage:
			if m.Event.ID != event.ID {
				t.Errorf("Unexpected event ID: %s", m.Event.ID)
			}

			if m.Event.Content != event.Content {
				t.Errorf("Unexpected event content: %s", m.Event.Content)
			}

			t.Logf("Received event: %s", m.Event.Content)

		case *relay.EOSEMessage:
			if m.SubscriptionID != req.SubscriptionID {
				t.Fatalf("Unexpected EOSE subscription ID: %s", m.SubscriptionID)
			}

			t.Logf("EOSE received: %s", m.SubscriptionID)
			return
		}
	}
}

// Tests that a subscription continues receiving events after EOSE. Client A subscribes to Client B, receives EOSE,
// then receives a new event published by Client B without creating another subscription. Note that the valid
// output would look like: Client A subscribes (receives nothing) --> Client B publishes --> Client A receives.
func TestSimulateClientsAndReceiveLiveEventsAfterEOSE(t *testing.T) {
	liveEventSent := false // Just here to prevent an inf loop

	// 0. Generate a valid keypair for Client B
	privKey, pubKey := generateKeypair(t)

	// 1. Establish independent relay connections for Client A and B
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	clientA, err := relay.Connect(ctx, "wss://relay.primal.net") // Client A
	if err != nil {
		t.Fatalf("Client A failed to connect: %v", err)
	}

	t.Log("Client A connected to relay!")
	defer clientA.Close()

	clientB, err := relay.Connect(ctx, "wss://relay.primal.net") // Client B
	if err != nil {
		t.Fatalf("Client B failed to connect: %v", err)
	}

	t.Log("Client B connected to relay!")
	defer clientB.Close()

	// 2. Client A subscribes to Client B's Kind 1 events
	req := relay.ReqMessage{
		SubscriptionID: "live-test",
		Filters: []relay.Filter{
			{
				"authors": []string{pubKey}, // Client A uses Client B's Public Key to subscribe
				"kinds":   []int{1},
			},
		},
	}

	if err := clientA.Subscribe(ctx, req); err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// 3. Wait until Client A receives EOSE
	for {
		msg, err := clientA.ReceiveEventSync(ctx)
		if err != nil {
			t.Fatalf("Failed to receive response from relay: %v", err)
		}

		switch m := msg.(type) {
		case *relay.IncomingEventMessage:
			t.Logf("Client A receives event:  %s", m.Event.ID)

			// 5. (The ordering is fine, dw) Sends a CLOSE message to the relay to terminate subscription
			if liveEventSent && m.Event.Content == "Live event after EOSE!" {
				if err := clientA.CloseSubscription(ctx, req.SubscriptionID); err != nil {
					t.Fatalf("Client A failed to close subscription: %v", err)
				}

				t.Log("Client A terminates subscription")

				// 6. Properly terminates websocket connections
				if err := clientA.Close(); err != nil {
					t.Fatalf("Client A failed to close websocket connection: %v", err)
				}

				t.Log("Client A terminates websocket connection")

				if err := clientB.Close(); err != nil {
					t.Fatalf("Client A failed to close websocket connection: %v", err)
				}

				t.Log("Client B terminates websocket connection")
				return
			}

		case *relay.EOSEMessage:

			// NOTE: Typically EOSE messages are received only once which means that the relay has finished
			//	     sending any stored events matching the REQ filters before the subscription was established. Now
			//       all other messages use the open websocket connection and received directly with no EOSE acting as
			// 		 a terminator.

			if liveEventSent {
				continue
			}

			if m.SubscriptionID != req.SubscriptionID {
				t.Fatalf("Unexpected EOSE subscription ID: %s", m.SubscriptionID)
			}

			t.Log("Client A received EOSE! Subscription remains active")

			// 4. Client B publishes a new event after EOSE to demonstrate Client A's ability to receive
			//    live events without a new subscription

			liveEventSent = true
			event := nostr.Event{
				PubKey:    pubKey, // Client B's public key (I could have just made the name more indicative of that)
				CreatedAt: time.Now().Unix(),
				Kind:      1,
				Tags:      [][]string{},
				Content:   "Live event after EOSE!",
			}

			if err := event.Sign(privKey); err != nil {
				t.Fatal(err)
			}

			ack, err := clientB.Publish(ctx, &event)
			if err != nil {
				t.Fatalf("Failed publishing live event: %v", err)
			}

			if !ack.Accepted {
				t.Fatalf("Live event rejected: %s", ack.Message)
			}

			t.Logf("Client B published event: %s", event.ID)
		}
	}
}
