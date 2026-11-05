# 📨 Nostr CLI Chat App
A simple CLI messaging app over NIP-01 with Nostr protocol implemented from scratch for educational purposes. I am well aware of the security concerns messaging over NIP-01 raises and as I said, this was to understand how underlying Nostr
protocol works.

I suggest reading the official docs at:
- [Nostr Dev Guide](https://nostrcg.github.io/devguide/getting-started/introduction/)
- [NIPS GitHub](https://github.com/nostr-protocol/nips/blob/master/01.md)

## 📦 Prerequisites

- [Go](https://go.dev/dl/) 1.27 or newer
- A reachable Nostr relay (defaults to `wss://relay.primal.net`)

## 🔧 How it works?

- Connect to a relay over WebSocket.
- Each peer sends a `REQ` for `kind-1` events authored by the remote public key and addressed to its own key (`#p`). This is the reverse REQ: each side subscribes to the other side's outgoing messages.
- Messages are published as signed `kind-1` EVENTs with a `p` tag for the peer, and the relay confirms with an `ACK`.
- Incoming events are validated (author, tag, signature) before being printed; `EOSE` and unrelated events are ignored.
- On exit, the subscription is closed with `CLOSE` and the relay connection is shut down.

## 💻 Development: Setup and Usage

Follow these steps to get your development environment setup.

1. **Clone the repository**
    ```bash
    git clone https://github.com/dotping-me/nostr-cli-chat.git
    cd nostr-cli-chat
    ```

2. **Install dependencies**
    ```bash
    go mod tidy
    ```

3. **Run tests**
    ```bash
    go test ./...
    ```

4. **Chat**  
    Start one instance per peer (on seperate terminals). Without flags, each instance prints its public key and prompts for the other peer's key:

    ```bash
    go run ./cmd/nostr-cli-chat chat
    ```

## ☢️ Danger!
Keys in this implementation are meant to be disposable and messages are plaintext. **Do not use real Nostr keys!**
