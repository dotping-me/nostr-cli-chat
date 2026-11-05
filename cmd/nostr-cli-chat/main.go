package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/dotping-me/nostr-cli-chat/internal/chat"
	"github.com/dotping-me/nostr-cli-chat/internal/nostr"
	"github.com/dotping-me/nostr-cli-chat/internal/relay"
	"github.com/spf13/cobra"
)

const defaultRelayURL = "wss://relay.primal.net"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "nostr-cli-chat",
		Short:         "Minimal p2p chat over NIP-01 relays",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newKeysCmd(), newChatCmd())
	return root
}

func newKeysCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "keys",
		Short: "Generate and display a disposable keypair",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, privHex, pubHex, err := generateKeys()
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Private key: %s\n", privHex)
			fmt.Fprintf(cmd.OutOrStdout(), "Public key:  %s\n", pubHex)
			return nil
		},
	}
}

func newChatCmd() *cobra.Command {
	var relayURL, peer, privateKeyHex string

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Connect to a peer and exchange messages",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runChat(cmd, relayURL, peer, privateKeyHex)
		},
	}

	cmd.Flags().StringVar(&relayURL, "relay", defaultRelayURL, "relay websocket URL")
	cmd.Flags().StringVar(&peer, "peer", "", "peer x-only public key (hex); prompted when empty")
	cmd.Flags().StringVar(&privateKeyHex, "private-key", "", "private key hex (from the keys command); generated when empty")
	return cmd
}

func runChat(cmd *cobra.Command, relayURL, peer, privateKeyHex string) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	privateKey, privHex, pubHex, err := loadOrGenerateKeys(privateKeyHex)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Your private key: %s\n", privHex)
	fmt.Fprintf(out, "Your public key:  %s\n", pubHex)

	// The same reader is passed to the conversation so that a peer key read here does not swallow
	// bytes that the chat loop would otherwise need.
	reader := bufio.NewReader(os.Stdin)
	if peer == "" {
		fmt.Fprint(out, "Peer public key: ")

		type promptResult struct {
			line string
			err  error
		}

		result := make(chan promptResult, 1)
		go func() {
			line, err := reader.ReadString('\n')
			result <- promptResult{line: line, err: err}
		}()

		select {
		case <-ctx.Done():
			return nil

		case res := <-result:
			if res.err != nil {
				return fmt.Errorf("read peer public key: %w", res.err)
			}

			peer = strings.TrimSpace(res.line)
		}
	}

	if _, err := nostr.ValidateAndReturnSchnorrPublicKey(peer); err != nil {
		return fmt.Errorf("invalid peer public key: %w", err)
	}

	fmt.Fprintf(out, "Connecting to %s...\n", relayURL)

	r, err := relay.ConnectAsync(ctx, relayURL)
	if err != nil {
		return fmt.Errorf("connect to relay: %w", err)
	}
	defer r.Close()

	conversation, err := chat.NewConversation(r, privateKey, peer)
	if err != nil {
		return fmt.Errorf("create conversation: %w", err)
	}

	if err := conversation.Run(ctx, reader, out); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}

		return fmt.Errorf("chat: %w", err)
	}

	return nil
}

func loadOrGenerateKeys(privateKeyHex string) (*btcec.PrivateKey, string, string, error) {
	if privateKeyHex == "" {
		return generateKeys()
	}

	raw, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid private key: %w", err)
	}

	if len(raw) != 32 {
		return nil, "", "", errors.New("private key must be 32 bytes (64 hex characters)")
	}

	privateKey, _ := btcec.PrivKeyFromBytes(raw)
	return privateKey,
		hex.EncodeToString(privateKey.Serialize()),
		hex.EncodeToString(schnorr.SerializePubKey(privateKey.PubKey())),
		nil
}

func generateKeys() (*btcec.PrivateKey, string, string, error) {
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, "", "", fmt.Errorf("generate private key: %w", err)
	}

	return privateKey,
		hex.EncodeToString(privateKey.Serialize()),
		hex.EncodeToString(schnorr.SerializePubKey(privateKey.PubKey())),
		nil
}
