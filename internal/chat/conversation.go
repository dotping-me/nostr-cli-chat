package chat

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
	"github.com/dotping-me/nostr-cli-chat/internal/relay"
)

// Conversation is a bidirectional, plaintext chat between this process and one
// remote public key over a single relay connection.
//
// A conversation is established in two steps:
//
//  1. NewConversation validates the local private key and the remote public
//     key, then derives the local public key. It performs no network I/O.
//  2. Subscribe registers a reverse REQ filter for events authored by the
//     remote key and tagged with the local key (#p). Each participant asks
//     for the other side's messages, so neither side receives its own
//     messages back.
//
// Once subscribed, Send publishes a signed kind-1 event carrying a "p" tag for
// the remote key, and Receive reads the matching events from the subscription.
// Send and Receive require an active subscription. Run wires them into an
// interactive terminal loop; Close tears the subscription down.
//
// Content is not encrypted and identity is not persisted. The local private
// key and remote public key are supplied by the caller and are expected to be
// disposable.
type Conversation struct {
	LocalPubKey  string
	RemotePubKey string

	relay        *relay.Relay
	privateKey   *btcec.PrivateKey
	subscription *relay.Subscription
}

// NewConversation creates a conversation over an already connected relay. It
// derives LocalPubKey from privateKey, checks that remotePubKey is a valid
// Schnorr public key, and rejects a conversation with the local key. The
// subscription is not started until Subscribe is called.
func NewConversation(r *relay.Relay, privateKey *btcec.PrivateKey, remotePubKey string) (*Conversation, error) {
	if r == nil {
		return nil, errors.New("relay cannot be nil")
	}

	if privateKey == nil {
		return nil, errors.New("private key cannot be nil")
	}

	if _, err := nostr.ValidateAndReturnSchnorrPublicKey(remotePubKey); err != nil {
		return nil, fmt.Errorf("invalid recipient public key: %w", err)
	}

	localPubKey := hex.EncodeToString(
		schnorr.SerializePubKey(privateKey.PubKey()),
	)

	if localPubKey == remotePubKey {
		return nil, errors.New("cannot start a conversation with yourself")
	}

	return &Conversation{LocalPubKey: localPubKey, RemotePubKey: remotePubKey, relay: r, privateKey: privateKey}, nil
}

// Subscribe starts receiving messages authored by the remote peer and
// addressed to this user. Each side sends its own REQ (the reverse REQ) so both
// directions are covered without receiving local echoes. Only one subscription
// may be active at a time.
func (c *Conversation) Subscribe(ctx context.Context) error {
	if c.subscription != nil {
		return errors.New("conversation is already subscribed")
	}

	var randomID [16]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		return fmt.Errorf("failed to generate subscription ID: %w", err)
	}

	// Reverse REQ meaning that user is currectly subscribing to events published by the peer's public key that
	// reference this user's public key

	subscriptionID := "chat-" + hex.EncodeToString(randomID[:])
	req := relay.ReqMessage{
		SubscriptionID: subscriptionID,
		Filters: []relay.Filter{
			{
				"kinds":   []int{1},
				"authors": []string{c.RemotePubKey},
				"#p":      []string{c.LocalPubKey},
			},
		},
	}

	sub, err := c.relay.SubscribeAsync(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to subscribe to conversation: %w", err)
	}

	c.subscription = sub
	return nil
}

// Send builds a kind-1 event tagged with the remote key, signs it with the
// local private key, publishes it, and waits for the relay ACK. A missing ACK
// or a rejected event is returned as an error.
func (c *Conversation) Send(ctx context.Context, content string) error {
	if c.subscription == nil {
		return errors.New("conversation is not subscribed")
	}

	event := &nostr.Event{
		PubKey:    c.LocalPubKey,
		CreatedAt: time.Now().Unix(),
		Kind:      1,
		Tags: [][]string{
			{"p", c.RemotePubKey},
		},
		Content: content,
	}

	if err := event.Sign(c.privateKey); err != nil {
		return fmt.Errorf("failed to sign message: %w", err)
	}

	ackCh, err := c.relay.PublishAsync(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	select {
	case ack, ok := <-ackCh:
		if !ok {
			return errors.New("relay disconnected before acknowledging message")
		}

		if !ack.Accepted {
			return fmt.Errorf("relay rejected message: %s", ack.Message)
		}

		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}

// Receive blocks until a valid message arrives or ctx is cancelled. It skips
// EOSE markers and events that fail validation, so only events authored by the
// remote key and addressed to the local key are returned.
func (c *Conversation) Receive(ctx context.Context) (*Message, error) {
	if c.subscription == nil {
		return nil, errors.New("conversation is not subscribed")
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case response, ok := <-c.subscription.Messages:
			if !ok {
				return nil, errors.New("relay subscription closed")
			}

			switch msg := response.(type) {
			case *relay.IncomingEventMessage:
				message, err := MessageFromEvent(
					&msg.Event,
					c.LocalPubKey,
					c.RemotePubKey,
				)

				if err != nil {
					continue
				}

				return message, nil

			case *relay.EOSEMessage:
				// Historical events have been exhausted.
				// Keep listening for live events.
				continue
			}
		}
	}
}

// Terminates the conversation subscription.
func (c *Conversation) Close(ctx context.Context) error {
	if c.subscription == nil {
		return nil
	}

	err := c.relay.CloseSubscription(ctx, c.subscription.ID)
	if err == nil {
		c.subscription = nil
	}

	return err
}

// Run subscribes, then drives an interactive terminal chat session until
// /quit, EOF, context cancellation, or relay disconnection.
//
// It uses two producer goroutines feeding a select loop:
//   - the input goroutine reads lines from input and reports scanner errors;
//   - the receive goroutine calls Receive and forwards valid messages.
//
// The select loop multiplexes incoming messages, outgoing lines, input EOF and
// receiver errors. On return, Run cancels runCtx so the producers stop
// forwarding, waits for Receive to finish, and only then closes the
// subscription.
func (c *Conversation) Run(ctx context.Context, input io.Reader, output io.Writer) error {
	if err := c.Subscribe(ctx); err != nil {
		return err
	}

	// runCtx is cancelled on every return path so the producers stop forwarding
	// before the deferred cleanup closes the subscription.
	runCtx, cancel := context.WithCancel(ctx)
	receiveDone := make(chan struct{})

	defer func() {
		cancel()
		<-receiveDone // Stop the receiver before Close mutates the subscription.

		closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer closeCancel()

		_ = c.Close(closeCtx)
	}()

	fmt.Fprintf(output, "Connected to conversation with %s\n", c.RemotePubKey)
	fmt.Fprintln(output, "Type /quit to exit.")

	inputLines := make(chan string)
	inputErrors := make(chan error, 1)

	// Read input on a goroutine so the select loop can keep printing incoming
	// messages while it waits for the next line.
	go func() {
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 4096), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()

			select {
			case inputLines <- line:
			case <-runCtx.Done():
				return
			}
		}

		inputErrors <- scanner.Err()
		close(inputLines)
	}()

	inbound := make(chan *Message)
	receiveErrors := make(chan error, 1)

	// Receive blocks on the subscription, so run it on its own goroutine and
	// forward valid messages to the select loop.
	go func() {
		defer close(receiveDone)

		for {
			message, err := c.Receive(runCtx)
			if err != nil {
				receiveErrors <- err
				return
			}

			select {
			case inbound <- message:
			case <-runCtx.Done():
				return
			}
		}
	}()

	// The select loop owns the session lifetime. It exits on /quit, EOF,
	// cancellation, or a receiver error; the deferred cleanup then cancels
	// runCtx and closes the subscription.
	for {
		select {
		case <-runCtx.Done():
			return ctx.Err()

		case err := <-receiveErrors:
			if errors.Is(err, context.Canceled) {
				return nil
			}

			return err

		case message := <-inbound:
			fmt.Fprintln(output, FormatMessage(message))

		case err := <-inputErrors:
			if err != nil {
				return fmt.Errorf("failed to read input: %w", err)
			}

			return nil

		case line, ok := <-inputLines:
			if !ok {
				return nil
			}

			if line == "/quit" {
				return nil
			}

			if line == "" {
				continue
			}

			if err := c.Send(runCtx, line); err != nil {
				fmt.Fprintf(output, "Failed to send: %v\n", err)
				continue
			}

			fmt.Fprintf(output, "You: %s\n", line)
		}
	}
}

// Optional helper for displaying a received message.
func FormatMessage(message *Message) string {
	timestamp := time.Unix(message.CreatedAt, 0).Format("15:04:05")

	sender := message.Sender
	if len(sender) > 12 {
		sender = sender[:12]
	}

	return fmt.Sprintf("[%s] %s: %s", timestamp, sender, message.Content)
}
