package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
)

type Box struct{ aead cipher.AEAD }

func New(key string) (*Box, error) {
	raw, e := hex.DecodeString(key)
	if e != nil || len(raw) != 32 {
		return nil, errors.New("LUMA_MASTER_KEY must contain 64 hexadecimal characters")
	}
	block, e := aes.NewCipher(raw)
	if e != nil {
		return nil, e
	}
	aead, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	return &Box{aead}, nil
}
func (b *Box) Seal(plain []byte, record string) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, e := rand.Read(nonce); e != nil {
		return nil, e
	}
	return b.aead.Seal(nonce, nonce, plain, []byte("luma:v1:"+record)), nil
}
func (b *Box) Open(data []byte, record string) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(data) < n {
		return nil, errors.New("invalid encrypted record")
	}
	plain, e := b.aead.Open(nil, data[:n], data[n:], []byte("luma:v1:"+record))
	if e != nil {
		return nil, errors.New("cannot decrypt record; verify LUMA_MASTER_KEY")
	}
	return plain, nil
}
