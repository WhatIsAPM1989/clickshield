package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type Cipher struct{ aead cipher.AEAD }

func New(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}
func (c *Cipher) EncryptJSON(v any) ([]byte, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return append(nonce, c.aead.Seal(nil, nonce, plain, nil)...), nil
}
func (c *Cipher) DecryptJSON(data []byte, dst any) error {
	if len(data) < c.aead.NonceSize() {
		return errors.New("encrypted value is too short")
	}
	plain, err := c.aead.Open(nil, data[:c.aead.NonceSize()], data[c.aead.NonceSize():], nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(plain, dst)
}
