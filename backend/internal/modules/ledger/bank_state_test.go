package ledger

import (
	"encoding/base64"
	"errors"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"testing"
)

var (
	bankTestKey      = []byte("0123456789abcdef0123456789abcdef")
	bankTestOtherKey = []byte("fedcba9876543210fedcba9876543210")
)

func TestBankAuthState_RoundTripAndEntropy(t *testing.T) {
	a, err := newBankAuthState("user-1", bankTestKey)
	if err != nil {
		t.Fatal(err)
	}
	b, err := newBankAuthState("user-1", bankTestKey)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two states are equal")
	}
	// 16 random payload bytes + 12 nonce bytes + 16 tag bytes, base64 encoded twice.
	if len(a) < 64 {
		t.Fatalf("state too short: %d", len(a))
	}
	if _, err := base64.RawURLEncoding.DecodeString(a); err != nil {
		t.Fatalf("state is not URL-safe base64: %v", err)
	}
	userID, err := openBankAuthState(a, bankTestKey)
	if err != nil || userID != "user-1" {
		t.Fatalf("open = %q, %v", userID, err)
	}
	if bankStateHash(a) == bankStateHash(b) || len(bankStateHash(a)) != 64 {
		t.Fatal("state hash must be unique and hex sha256")
	}
}

func TestBankAuthState_RejectsInvalid(t *testing.T) {
	valid, err := newBankAuthState("user-1", bankTestKey)
	if err != nil {
		t.Fatal(err)
	}
	forged := []byte(valid)
	forged[len(forged)/2] ^= 0x01
	tests := map[string]string{
		"empty":     "",
		"garbled":   "!!!not-base64!!!",
		"plain":     base64.RawURLEncoding.EncodeToString([]byte("user-1.00112233445566778899aabbccddeeff")),
		"tampered":  string(forged),
		"truncated": valid[:len(valid)-8],
	}
	for name, state := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := openBankAuthState(state, bankTestKey); !errors.Is(err, ErrBankStateInvalid) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	t.Run("other key", func(t *testing.T) {
		if _, err := openBankAuthState(valid, bankTestOtherKey); !errors.Is(err, ErrBankStateInvalid) {
			t.Fatalf("err = %v", err)
		}
	})
}

func encryptBankTestSession(plain string) (string, error) {
	return cryptoutil.EncryptString(plain, bankTestKey)
}
