package control

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const credentialKeyVersion = 1

type credentialVault struct {
	aead cipher.AEAD
}

type credentialSecret struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func newCredentialVault(encodedKey string) (*credentialVault, error) {
	encodedKey = strings.TrimSpace(encodedKey)
	if encodedKey == "" {
		return nil, errors.New("vault key is not configured")
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, errors.New("vault key must be base64 encoded")
	}
	defer clear(key)
	if len(key) != 32 {
		return nil, errors.New("vault key must decode to 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create vault cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create vault AEAD: %w", err)
	}
	return &credentialVault{aead: aead}, nil
}

func (v *credentialVault) encrypt(ownerID, origin string, secret credentialSecret) (string, error) {
	if v == nil || v.aead == nil {
		return "", errors.New("vault is unavailable")
	}
	plaintext, err := json.Marshal(secret)
	if err != nil {
		return "", fmt.Errorf("encode credential: %w", err)
	}
	defer clear(plaintext)
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("create credential nonce: %w", err)
	}
	sealed := v.aead.Seal(nonce, nonce, plaintext, credentialAAD(ownerID, origin))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (v *credentialVault) decrypt(ownerID, origin, encodedPayload string) (credentialSecret, error) {
	if v == nil || v.aead == nil {
		return credentialSecret{}, errors.New("vault is unavailable")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encodedPayload))
	if err != nil || len(payload) <= v.aead.NonceSize() {
		return credentialSecret{}, errors.New("credential payload is invalid")
	}
	nonce, ciphertext := payload[:v.aead.NonceSize()], payload[v.aead.NonceSize():]
	plaintext, err := v.aead.Open(nil, nonce, ciphertext, credentialAAD(ownerID, origin))
	if err != nil {
		return credentialSecret{}, errors.New("credential payload authentication failed")
	}
	defer clear(plaintext)
	var secret credentialSecret
	if err := json.Unmarshal(plaintext, &secret); err != nil {
		return credentialSecret{}, errors.New("credential payload is invalid")
	}
	return secret, nil
}

func credentialAAD(ownerID, origin string) []byte {
	return []byte("navego:credential:v1:" + strings.TrimSpace(ownerID) + ":" + strings.TrimSpace(origin))
}
