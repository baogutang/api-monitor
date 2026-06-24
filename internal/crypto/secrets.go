package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

type Service struct {
	key [32]byte
}

func New(secret string) Service {
	return Service{key: sha256.Sum256([]byte(secret))}
}

func (s Service) Encrypt(plain []byte) (string, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, plain, nil)
	payload := append(nonce, ciphertext...)
	return base64.RawStdEncoding.EncodeToString(payload), nil
}

func (s Service) Decrypt(encoded string) ([]byte, error) {
	payload, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(payload) < gcm.NonceSize() {
		return nil, fmt.Errorf("secret payload too short")
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func Fingerprint(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

func NewID(prefix string) string {
	var b [16]byte
	_, _ = io.ReadFull(rand.Reader, b[:])
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b[:]))
}
