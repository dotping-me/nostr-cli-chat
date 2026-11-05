package main

import (
	"context"
	"fmt"
	"log"

	"github.com/coder/websocket"
)

// Just a test to see if I'm able to connect to an open nostr relay instead of having to implement and
// host one myself

func main() {
	ctx := context.Background()

	conn, _, err := websocket.Dial(ctx, "wss://relay.primal.net", nil)
	if err != nil {
		log.Fatal(err)
	}

	defer conn.Close(websocket.StatusNormalClosure, websocket.StatusNormalClosure.String())
	fmt.Println("Connected to relay!")

	_, msg, err := conn.Read(ctx) // Read a message from the relay
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Received: %s\n", msg)
}
