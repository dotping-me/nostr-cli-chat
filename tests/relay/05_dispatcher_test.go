package relay_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
	"github.com/dotping-me/nostr-cli-chat/internal/relay"
)

// Implements and tests single read process/connection using dispactcher which routes inbound traffic to appropriate
// dispatchers.
func TestDispatcherReceivesLiveEvent(t *testing.T) {

	// 0. Generates a valid keypair
	privKey, pubKey := generateKeypair(t)

	// 1. Connects to relay with dispatcher architecture
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r, err := relay.ConnectAsync(ctx, "wss://relay.primal.net")
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}

	// 2. Creates and send REQ
	req := relay.ReqMessage{
		SubscriptionID: "test-sub",
		Filters: []relay.Filter{
			{
				"authors": []string{pubKey},
				"kinds":   []int{1},
			},
		},
	}

	sub, err := r.SubscribeAsync(ctx, req)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// 3. Waits for EOSE before continuing (blocking)
	select {
	case msg := <-sub.Messages: // sub.Messages is unique to that subscription instance
		if msg.Type() != relay.EOSE_CODE {
			t.Fatalf("Expected %s but received %s", relay.EOSE_CODE, msg.Type())
		}

		t.Logf("Received EOSE  for Subscription ID: %s", sub.ID)

	case <-ctx.Done():
		t.Fatalf("Timed out waiting for %s", relay.EOSE_CODE)
	}

	// 4. Creates and publishes an event satisfying subscription
	event := &nostr.Event{
		PubKey:    pubKey,
		CreatedAt: time.Now().Unix(),
		Kind:      1,
		Tags:      [][]string{},
		Content:   "Dispatcher integration test",
	}

	if err := event.Sign(privKey); err != nil {
		t.Fatalf("Failed to sign event: %v", err)
	}

	ackCh, err := r.PublishAsync(ctx, event)
	if err != nil {
		t.Fatalf("Failed to publish event: %v", err)
	}

	// Waits for ACK first
	select {
	case ack, ok := <-ackCh:
		if !ok {
			t.Fatal("Relay disconnected before ACK")
		}

		if !ack.Accepted {
			t.Fatalf("Failed to ACK")
		}

		t.Logf("Received ACK")

	case <-ctx.Done():
		t.Fatalf("Timed out waiting for ACK: id=%s", event.ID)
	}

	// 5. Verifies dispatcher delivers the live event
	select {
	case msg := <-sub.Messages:
		incoming, ok := msg.(*relay.IncomingEventMessage)
		if !ok {
			t.Fatalf("Expected %s but received %s", relay.EVENT_CODE, msg.Type())
		}

		if incoming.Event.ID != event.ID {
			t.Fatalf("Expected event %s, got %s", event.ID, incoming.Event.ID)
		}

		t.Logf("Received EVENT for Subscription ID: %s (\"%s\")", incoming.SubscriptionID, incoming.Event.Content)

	case <-ctx.Done():
		t.Fatal("Timed out waiting for live event")
	}
}

// This is the whole point behind using the dispatcher so that sending and receiving can happen at the same
// time without one blocking the other or having to be paused. Note that this is a simulation and typically this
// would happen because of how users communicate.
func TestDispatcherConcurrentPublishAndReceive(t *testing.T) {

	// 0. Generates a valid keypair
	privKey, pubKey := generateKeypair(t)

	// 1. Connects to relay with dispatcher architecture
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r, err := relay.ConnectAsync(ctx, "wss://relay.primal.net")
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}

	// 2. Creates and send REQ
	req := relay.ReqMessage{
		SubscriptionID: "test-sub",
		Filters: []relay.Filter{
			{
				"authors": []string{pubKey},
				"kinds":   []int{1},
			},
		},
	}

	sub, err := r.SubscribeAsync(ctx, req)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// 3. Waits for EOSE before continuing (blocking)
	select {
	case msg := <-sub.Messages: // sub.Messages is unique to that subscription instance
		if msg.Type() != relay.EOSE_CODE {
			t.Fatalf("Expected %s but received %s", relay.EOSE_CODE, msg.Type())
		}

		t.Logf("Received EOSE  for Subscription ID: %s", sub.ID)

	case <-ctx.Done():
		t.Fatalf("Timed out waiting for %s", relay.EOSE_CODE)
	}

	// 4. Independent process that consumes incound events while another process is publishing events to the relay
	const count = 5
	received := make(chan string, count)

	go func() {
		for i := 0; i < count; {
			select {
			case msg := <-sub.Messages:
				incoming, ok := msg.(*relay.IncomingEventMessage)
				if !ok {
					continue
				}

				t.Logf("Received EVENT for Subcription ID: %s (\"%s\")", sub.ID, incoming.Event.Content)
				received <- incoming.Event.Content
				i++

			case <-ctx.Done():
				return
			}
		}
	}()

	// 5. The other process continuously publishing events to relay concurrently
	t.Logf("Publishing %d events concurrently...", count)
	ackChannels := make([]<-chan *relay.OKMessage, count)

	for i := 0; i < count; i++ {
		event := &nostr.Event{
			PubKey:    pubKey,
			CreatedAt: time.Now().Unix(),
			Kind:      1,
			Tags:      [][]string{},
			Content:   fmt.Sprintf("Concurrent event %d", i),
		}

		if err := event.Sign(privKey); err != nil {
			t.Fatalf("Failed to sign event %d: %v", i, err)
		}

		ackCh, err := r.PublishAsync(ctx, event)
		if err != nil {
			t.Fatalf("Failed to publish event %d: %v", i, err)
		}

		ackChannels[i] = ackCh
		t.Logf("Event sent: %s", event.ID)
	}

	// 6. Verifies every publication receives an ACK
	for i, ackCh := range ackChannels {
		select {
		case ack, ok := <-ackCh:
			if !ok {
				t.Fatal("Relay disconnected before ACK")
			}

			if !ack.Accepted {
				t.Errorf("Event %d rejected: %s", i, ack.Message)
			}

			t.Logf("Received ACK: event=%d accepted=%t", i, ack.Accepted)

		case <-ctx.Done():
			t.Fatalf("Timed out waiting for ACK: event=%d", i)
		}
	}

	// 7. Verifies every published event reaches the subscription
	for i := 0; i < count; i++ {
		select {
		case <-received:
		case <-ctx.Done():
			t.Fatalf("Received only %d/%d events", i, count)
		}
	}

	t.Log("Dispatcher concurrency test completed successfully")
}
