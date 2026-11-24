package gmail

import (
	"testing"
)

func TestEncryptDecryptAESGCM(t *testing.T) {
	key := "my-super-secret-key-for-oauth"
	original := "1//04test_refresh_token_xyz_123456789"

	encrypted, err := EncryptAESGCM(original, key)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if encrypted == original {
		t.Errorf("ciphertext should not match plaintext")
	}

	decrypted, err := DecryptAESGCM(encrypted, key)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if decrypted != original {
		t.Errorf("expected decrypted text %q, got %q", original, decrypted)
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	original := "secret_token_123"
	encrypted, err := EncryptAESGCM(original, "correct-key")
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	_, err = DecryptAESGCM(encrypted, "wrong-key")
	if err == nil {
		t.Errorf("expected error when decrypting with wrong key, got nil")
	}
}

func TestEmptyPlaintext(t *testing.T) {
	encrypted, err := EncryptAESGCM("", "key")
	if err != nil {
		t.Fatalf("unexpected error on empty: %v", err)
	}
	if encrypted != "" {
		t.Errorf("expected empty string on empty input")
	}

	decrypted, err := DecryptAESGCM("", "key")
	if err != nil {
		t.Fatalf("unexpected error on empty: %v", err)
	}
	if decrypted != "" {
		t.Errorf("expected empty string on empty input")
	}
}
