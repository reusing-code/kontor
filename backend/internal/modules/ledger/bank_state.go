package ledger

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
)

const bankAuthStateTTL = 15 * time.Minute

var ErrBankStateInvalid = errors.New("bank authorization state is invalid")

// BankAuthState is the pending consent request stored under its state hash.
type BankAuthState struct {
	ASPSPName    string    `json:"aspspName"`
	ASPSPCountry string    `json:"aspspCountry"`
	CreatedAt    time.Time `json:"createdAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

// newBankAuthState returns a URL-safe state: the AES-GCM encrypted user ID plus
// 128 random bits. The callback is public, so the state is its only credential;
// it is unforgeable without the key and does not reveal the user ID to the bank.
func newBankAuthState(userID string, key []byte) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", errors.New("generating state failed")
	}
	enc, err := cryptoutil.EncryptString(userID+"."+hex.EncodeToString(nonce), key)
	if err != nil {
		return "", errors.New("encrypting state failed")
	}
	return base64.RawURLEncoding.EncodeToString([]byte(enc)), nil
}

// openBankAuthState returns the user ID inside a state, or ErrBankStateInvalid
// for anything garbled, forged or made with another key.
func openBankAuthState(state string, key []byte) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil {
		return "", ErrBankStateInvalid
	}
	plain, err := cryptoutil.DecryptString(string(raw), key)
	if err != nil {
		return "", ErrBankStateInvalid
	}
	i := strings.LastIndex(plain, ".")
	if i <= 0 || len(plain)-i-1 != 32 {
		return "", ErrBankStateInvalid
	}
	return plain[:i], nil
}

func bankStateHash(state string) string {
	sum := sha256.Sum256([]byte(state))
	return hex.EncodeToString(sum[:])
}
