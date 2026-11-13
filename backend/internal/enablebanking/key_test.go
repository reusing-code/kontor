package enablebanking_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/enablebanking"
)

func writePEM(t *testing.T, typ string, der []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadPrivateKey(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecDER, err := x509.MarshalPKCS8PrivateKey(ecKey)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("pkcs1", func(t *testing.T) {
		got, err := enablebanking.LoadPrivateKey(writePEM(t, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(rsaKey)))
		if err != nil || !got.Equal(rsaKey) {
			t.Fatalf("err = %v, equal = %v", err, got != nil && got.Equal(rsaKey))
		}
	})
	t.Run("pkcs8", func(t *testing.T) {
		got, err := enablebanking.LoadPrivateKey(writePEM(t, "PRIVATE KEY", pkcs8))
		if err != nil || !got.Equal(rsaKey) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("ec rejected", func(t *testing.T) {
		if _, err := enablebanking.LoadPrivateKey(writePEM(t, "PRIVATE KEY", ecDER)); err == nil || !strings.Contains(err.Error(), "RSA") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("garbage not echoed", func(t *testing.T) {
		const secret = "SUPER-SECRET-KEY-CONTENT"
		path := filepath.Join(t.TempDir(), "key.pem")
		if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := enablebanking.LoadPrivateKey(path)
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("garbage inside pem not echoed", func(t *testing.T) {
		_, err := enablebanking.LoadPrivateKey(writePEM(t, "RSA PRIVATE KEY", []byte("SUPER-SECRET-DER")))
		if err == nil || strings.Contains(err.Error(), "SUPER-SECRET") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("missing file", func(t *testing.T) {
		if _, err := enablebanking.LoadPrivateKey(filepath.Join(t.TempDir(), "nope.pem")); err == nil {
			t.Fatal("expected error")
		}
	})
}
