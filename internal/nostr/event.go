package nostr

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

// Event payload and implementation as described by NIP-01
// See https://github.com/nostr-protocol/nips/blob/master/01.md for more information

// TODO: Implement proper abstraction/encapsulation for Event payload if that helps

type Event struct {
	ID        string     `json:"id"`
	PubKey    string     `json:"pubkey"`
	CreatedAt int64      `json:"created_at"`
	Kind      int        `json:"kind"`
	Tags      [][]string `json:"tags"`
	Content   string     `json:"content"`
	Sig       string     `json:"sig"`
}

// Utility function to validate a given public key and return the btcec equivalent for crypto operations (The key is
// being returning in hopes of reducing function calls).
func ValidateAndReturnSchnorrPublicKey(key string) (*btcec.PublicKey, error) {
	if len(key) != 64 { // 64 because hex are 2 characters each (i.e. FA E1 ...)
		return nil, fmt.Errorf("Public Key must be 32-byte hex")
	}

	pubKeyHex, err := hex.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("Invalid Public Key: %w", err)
	}

	pubKey, err := schnorr.ParsePubKey(pubKeyHex)
	if err != nil {
		return nil, fmt.Errorf("Public Key is invalid BIP-340 format: %w", err)
	}

	return pubKey, nil
}

func ValidateEventProperties(e *Event) error {
	if e == nil {
		return errors.New("Event cannot be nil")
	}

	if _, err := ValidateAndReturnSchnorrPublicKey(e.PubKey); err != nil {
		return err
	}

	if e.CreatedAt < 0 {
		return errors.New("Creation timestamp cannot be negative")
	}

	if e.Kind < 0 || e.Kind > 65535 {
		return errors.New("Kind must be between 0 and 65535 inclusive")
	}

	// TODO: Also validate Event ID and Signature, if applicable

	return nil
}

// Custom implementation to escape an Event's content string as per NIP-01 docs because standard JSON library
// escapes either all or no HTML characters while NIP-01 holds some considerations.
func EscapeAndSerializeContent(content string) ([]byte, error) {
	if !utf8.ValidString(content) {
		return nil, errors.New("Content must be valid UTF-8")
	}

	var buf bytes.Buffer
	buf.WriteByte('"')

	for i := 0; i < len(content); i++ {
		switch content[i] {
		case '\n':
			buf.WriteString(`\n`)
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		default:
			if content[i] < 0x20 {
				fmt.Fprintf(&buf, `\u%04x`, content[i])
			} else {
				buf.WriteByte(content[i])
			}
		}
	}

	buf.WriteByte('"')
	return buf.Bytes(), nil
}

func escapeJSONString(value string) ([]byte, error) {
	if !utf8.ValidString(value) {
		return nil, errors.New("String must be valid UTF-8")
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(value); err != nil {
		return nil, err
	}

	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// Utility function that serializes tags in the same fashion as escaping and serializing content but is its own
// implementation because of nested arrays.
func EscapeAndSerializeTags(tags [][]string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('[')

	for i, tag := range tags {
		if i > 0 {
			buf.WriteByte(',')
		}

		buf.WriteByte('[')
		for j, value := range tag {
			if j > 0 {
				buf.WriteByte(',')
			}

			encoded, err := escapeJSONString(value)
			if err != nil {
				return nil, err
			}

			buf.Write(encoded)
		}

		buf.WriteByte(']')
	}

	buf.WriteByte(']')
	return buf.Bytes(), nil
}

// Returns the NIP-01 Event Payload as a standard JSON array as per NIP-01 docs. The resulting UTF-8 encoded
// bytes will later be hashed using SHA-256.
func (e *Event) Serialize() ([]byte, error) {
	if e == nil {
		return nil, errors.New("Event cannot be nil")
	}

	if err := ValidateEventProperties(e); err != nil {
		return nil, err
	}

	content, err := EscapeAndSerializeContent(e.Content)
	if err != nil {
		return nil, err
	}

	tags, err := EscapeAndSerializeTags(e.Tags)
	if err != nil {
		return nil, err
	}

	// This follows structure as per NIP-01 docs: [0, PubKey, CreatedAt, Kind, Tags, Contents]
	var buf bytes.Buffer

	buf.WriteString("[0,")
	buf.WriteString(`"` + e.PubKey + `"`)
	buf.WriteByte(',')
	buf.WriteString(strconv.FormatInt(e.CreatedAt, 10))
	buf.WriteByte(',')
	buf.WriteString(strconv.Itoa(e.Kind))
	buf.WriteByte(',')
	buf.Write(tags)
	buf.WriteByte(',')
	buf.Write(content)
	buf.WriteByte(']')

	return buf.Bytes(), nil
}

// Computes an Event's ID but hashing its serialized payload making that id unique to that payload only.
func (e *Event) ComputeID() (string, error) {
	if e == nil {
		return "", errors.New("Event cannot be nil")
	}

	serialized, err := e.Serialize()
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(serialized)
	return hex.EncodeToString(hash[:]), nil
}

// Computes the event ID and signs it using BIP-340 Schnorr.
func (e *Event) Sign(privateKey *btcec.PrivateKey) error {
	if e == nil {
		return errors.New("Event cannot be nil")
	}

	if privateKey == nil {
		return errors.New("Private Key cannot be nil")
	}

	// Derive the resulting Public Key from the provided Private Key and compare it against the Event's
	// Public Key to ensure integrity
	pubKeyHex := hex.EncodeToString(schnorr.SerializePubKey(privateKey.PubKey()))
	if pubKeyHex != e.PubKey {
		return errors.New("Private Key does not belong to event's Public Key")
	}

	// Computes hashed ID, converts hashed back into raw bytes which is used for signature. The hashed ID and signature
	// are then both attached to the payload.

	id, err := e.ComputeID()
	if err != nil {
		return err
	}

	idBytes, err := hex.DecodeString(id)
	if err != nil {
		return err
	}

	signature, err := schnorr.Sign(privateKey, idBytes) // Signs raw bytes using the private key
	if err != nil {
		return err
	}

	e.ID = id
	e.Sig = hex.EncodeToString(signature.Serialize())

	return nil
}

// Verifies an event's integrity by recomputing the ID and comparing it with the ID in the payload.
func (e *Event) verifyID() error {
	if e == nil {
		return errors.New("Event cannot be nil")
	}

	computedID, err := e.ComputeID() // Recomputes ID
	if err != nil {
		return fmt.Errorf("Failed to compute Event ID during ID verification: %w", err)
	}

	if computedID != e.ID {
		return errors.New("Event ID does not match computed ID")
	}

	return nil
}

// Verifies the Schnorr signature of the event payload against event ID and public key in payload. This function
// assumes that ID in payload is valid.
func (e *Event) verifySignature() error {
	if e == nil {
		return errors.New("Event cannot be nil")
	}

	// Decode ID and signature into raw bytes from hex representation
	idBytes, err := hex.DecodeString(e.ID)
	if err != nil {
		return fmt.Errorf("Invalid Event ID hex during Signature verification: %w", err)
	}

	sigBytes, err := hex.DecodeString(e.Sig)
	if err != nil {
		return fmt.Errorf("Invalid Event signature hex during Signature verification: %w", err)
	}

	// Attempts to parse signature
	signature, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		return fmt.Errorf("Failed to parse Event signature during Signature verification: %w", err)
	}

	// Validates the provided public key in the event's payload and verifies if that same key mathes the
	// event's signature

	pubKey, err := ValidateAndReturnSchnorrPublicKey(e.PubKey)
	if err != nil {
		return fmt.Errorf("Failed to parse Public Keu ")
	}

	if !signature.Verify(idBytes, pubKey) {
		return errors.New("Invalid event signature")
	}

	return nil
}

// General method for verifying Event integrity and authenticity.
func (e *Event) Verify() error {
	if e == nil {
		return errors.New("Event cannot be nil")
	}

	if err := e.verifyID(); err != nil {
		return err
	}

	if err := e.verifySignature(); err != nil {
		return err
	}

	return nil
}
