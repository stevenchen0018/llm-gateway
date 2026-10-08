// Package idgen generates request identifiers and API key secrets.
package idgen

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
)

// RequestID returns a new UUIDv4 suitable for request tracing.
func RequestID() string {
	return uuid.NewString()
}

// NewAPIKey generates a new API key secret in the form "sk-<40 hex chars>"
// plus a short, non-secret prefix (first 10 chars after "sk-") used for fast
// lookup without needing to hash every stored key on every request.
func NewAPIKey() (secret string, prefix string, err error) {
	buf := make([]byte, 24)
	if _, err = rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate key entropy: %w", err)
	}
	secret = "sk-" + hex.EncodeToString(buf)
	prefix = secret[:10]
	return secret, prefix, nil
}

// HashAPIKey returns a deterministic, irreversible hash of an API key secret
// for storage/lookup; the plaintext secret is never persisted.
func HashAPIKey(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
