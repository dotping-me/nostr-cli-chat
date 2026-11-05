package relay

import (
	"context"
	"fmt"
	"sync"

	"github.com/coder/websocket"
)

// Relevant implementation regarding relays; connections, messaging and so on. Right now, I am using an
// open relay instead of implementing one.
//
// Every Relay runs the dispatcher (dispatch.go) as its single reader. The synchronous and asynchronous APIs in
// sync.go and async.go consume the channels produced by the dispatcher; neither reads from the websocket directly.

// Subscription receives every message routed by the dispatcher for a given subscription ID, in arrival order.
type Subscription struct {
	ID       string
	Messages chan RelayResponse
}

type Relay struct {
	URL  string
	conn *websocket.Conn

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{} // Closed once the dispatcher has stopped and cleaned up

	closeOnce sync.Once // Makes closing idempotent for both Sync and Async modes
	closeErr  error

	mu            sync.Mutex
	subscriptions map[string]*Subscription
	pending       map[string]chan *OKMessage // Pending ACKs for published events
	syncSub       *Subscription              // Subscription consumed by ReceiveEventSync
}

const (
	OK_CODE     = "OK"
	EVENT_CODE  = "EVENT"
	EOSE_CODE   = "EOSE"
	NOTICE_CODE = "NOTICE"
	CLOSE_CODE  = "CLOSE"
	REQ_CODE    = "REQ"
)

// Connects to a relay and starts the dispatcher that owns reading and routing.
func connect(ctx context.Context, url string) (*Relay, error) {
	relayCtx, cancel := context.WithCancel(ctx)
	conn, _, err := websocket.Dial(relayCtx, url, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("Failed to connect to relay (%s): %w", url, err)
	}

	r := &Relay{
		URL:           url,
		conn:          conn,
		ctx:           relayCtx,
		cancel:        cancel,
		done:          make(chan struct{}),
		subscriptions: make(map[string]*Subscription),
		pending:       make(map[string]chan *OKMessage),
	}

	go r.readLoop()
	return r, nil
}

// Sends a message through the relay.
func (r *Relay) Send(ctx context.Context, msg []byte) error {
	if err := r.conn.Write(ctx, websocket.MessageText, msg); err != nil {
		return fmt.Errorf("Failed to send message: %w", err)
	}

	return nil
}

// Sends a CLOSE message to the relay indicating a termination of a given subscription. The subscription channels
// remain open and are closed when the dispatcher stops.
func (r *Relay) CloseSubscription(ctx context.Context, subscriptionID string) error {
	msg, err := CloseMessage{SubscriptionID: subscriptionID}.MarshalJSON()
	if err != nil {
		return fmt.Errorf("Failed to encode %s: %w", CLOSE_CODE, err)
	}

	if err := r.Send(ctx, msg); err != nil {
		return fmt.Errorf("Failed to send %s: %w", CLOSE_CODE, err)
	}

	r.mu.Lock()
	if r.syncSub != nil && r.syncSub.ID == subscriptionID {
		r.syncSub = nil
	}
	r.mu.Unlock()

	return nil
}

// Terminates connection between client and relay and waits for the dispatcher to stop.
func (r *Relay) Close() error {
	r.closeOnce.Do(func() {
		r.cancel()
		r.closeErr = r.conn.Close(websocket.StatusNormalClosure, websocket.StatusNormalClosure.String())
		<-r.done
	})

	if r.closeErr != nil {
		return fmt.Errorf("Failed to close relay connection: %w", r.closeErr)
	}

	return nil
}
