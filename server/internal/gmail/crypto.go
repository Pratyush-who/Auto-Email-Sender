package gmail

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

func deriveKey(keyStr string) []byte {
	hash := sha256.Sum256([]byte(keyStr))
	return hash[:]
}

// EncryptAESGCM encrypts plaintext using AES-GCM with a key derived from keyStr.
// The returned string is hex-encoded and contains the 12-byte nonce prepended to the ciphertext.
func EncryptAESGCM(plaintext, keyStr string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if keyStr == "" {
		return "", errors.New("encryption key cannot be empty")
	}

	key := deriveKey(keyStr)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate random nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(sealed), nil
}

// DecryptAESGCM decrypts a hex-encoded string produced by EncryptAESGCM.
func DecryptAESGCM(encryptedHex, keyStr string) (string, error) {
	if encryptedHex == "" {
		return "", nil
	}
	if keyStr == "" {
		return "", errors.New("encryption key cannot be empty")
	}

	data, err := hex.DecodeString(encryptedHex)
	if err != nil {
		return "", fmt.Errorf("invalid hex encoding: %w", err)
	}

	key := deriveKey(keyStr)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt ciphertext: %w", err)
	}

	return string(plaintext), nil
}
