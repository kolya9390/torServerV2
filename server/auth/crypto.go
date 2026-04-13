package auth

import (
	"crypto/rand"
)

// readRandom fills the given slice with cryptographically secure random bytes.
func readRandom(b []byte) (int, error) {
	return rand.Read(b)
}
