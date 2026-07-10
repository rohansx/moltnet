package core

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/mr-tron/base58"
)

// Multicodec prefix for an Ed25519 public key (0xed) as an unsigned varint,
// per the did:key specification: [0xed, 0x01].
var ed25519MulticodecPrefix = []byte{0xed, 0x01}

// KeyPair is an Ed25519 identity. The public key, encoded as a did:key DID, is
// the permanent identifier for an agent or owner.
type KeyPair struct {
	DID     string
	Public  ed25519.PublicKey
	Private ed25519.PrivateKey
}

// GenerateKeyPair creates a fresh Ed25519 identity.
func GenerateKeyPair() (*KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &KeyPair{DID: DIDFromPublicKey(pub), Public: pub, Private: priv}, nil
}

// DIDFromPublicKey encodes an Ed25519 public key as a did:key DID.
func DIDFromPublicKey(pub ed25519.PublicKey) string {
	buf := make([]byte, 0, len(ed25519MulticodecPrefix)+len(pub))
	buf = append(buf, ed25519MulticodecPrefix...)
	buf = append(buf, pub...)
	// Multibase base58btc is prefixed with 'z'.
	return "did:key:z" + base58.Encode(buf)
}

// PublicKeyFromDID recovers the Ed25519 public key from a did:key DID.
func PublicKeyFromDID(did string) (ed25519.PublicKey, error) {
	const prefix = "did:key:z"
	if !strings.HasPrefix(did, prefix) {
		return nil, fmt.Errorf("did %q is not a did:key with base58btc encoding", did)
	}
	decoded, err := base58.Decode(strings.TrimPrefix(did, prefix))
	if err != nil {
		return nil, fmt.Errorf("did %q: base58 decode: %w", did, err)
	}
	if len(decoded) != len(ed25519MulticodecPrefix)+ed25519.PublicKeySize {
		return nil, fmt.Errorf("did %q: unexpected key length %d", did, len(decoded))
	}
	if decoded[0] != ed25519MulticodecPrefix[0] || decoded[1] != ed25519MulticodecPrefix[1] {
		return nil, fmt.Errorf("did %q: not an ed25519 multicodec key", did)
	}
	return ed25519.PublicKey(decoded[len(ed25519MulticodecPrefix):]), nil
}

// Sign produces a hex-encoded Ed25519 signature over msg.
func Sign(priv ed25519.PrivateKey, msg []byte) string {
	return hex.EncodeToString(ed25519.Sign(priv, msg))
}

// Verify checks a hex-encoded signature against a message and the public key
// recovered from the signer's DID.
func Verify(signerDID string, msg []byte, sigHex string) error {
	pub, err := PublicKeyFromDID(signerDID)
	if err != nil {
		return err
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return fmt.Errorf("signature is not valid hex: %w", err)
	}
	if !ed25519.Verify(pub, msg, sig) {
		return fmt.Errorf("signature does not verify for %s", signerDID)
	}
	return nil
}

// PrivateKeyHex / PublicKeyHex encode raw key bytes for keyfile storage.
func PrivateKeyHex(priv ed25519.PrivateKey) string { return hex.EncodeToString(priv) }
func PublicKeyHex(pub ed25519.PublicKey) string    { return hex.EncodeToString(pub) }

// KeyPairFromHex reconstructs a KeyPair from a hex-encoded private key seed or
// full private key.
func KeyPairFromHex(privHex string) (*KeyPair, error) {
	raw, err := hex.DecodeString(privHex)
	if err != nil {
		return nil, fmt.Errorf("private key is not valid hex: %w", err)
	}
	var priv ed25519.PrivateKey
	switch len(raw) {
	case ed25519.PrivateKeySize:
		priv = ed25519.PrivateKey(raw)
	case ed25519.SeedSize:
		priv = ed25519.NewKeyFromSeed(raw)
	default:
		return nil, fmt.Errorf("unexpected private key length %d", len(raw))
	}
	pub := priv.Public().(ed25519.PublicKey)
	return &KeyPair{DID: DIDFromPublicKey(pub), Public: pub, Private: priv}, nil
}
