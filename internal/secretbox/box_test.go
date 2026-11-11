package secretbox

import (
	"bytes"
	"strings"
	"testing"
)

func TestAuthenticatedEncryptionBoundToRecord(t *testing.T) {
	b, e := New(strings.Repeat("01", 32))
	if e != nil {
		t.Fatal(e)
	}
	plain := []byte("proxy credential")
	cipher, e := b.Seal(plain, "exit-one")
	if e != nil || bytes.Contains(cipher, plain) {
		t.Fatal("plaintext persisted", e)
	}
	other, _ := b.Seal(plain, "exit-one")
	if bytes.Equal(cipher, other) {
		t.Fatal("nonce reused")
	}
	got, e := b.Open(cipher, "exit-one")
	if e != nil || !bytes.Equal(plain, got) {
		t.Fatal(e)
	}
	if _, e = b.Open(cipher, "exit-two"); e == nil {
		t.Fatal("ciphertext moved across records")
	}
	cipher[len(cipher)-1] ^= 1
	if _, e = b.Open(cipher, "exit-one"); e == nil {
		t.Fatal("tampering accepted")
	}
	if _, e = New("short"); e == nil {
		t.Fatal("invalid key accepted")
	}
}
