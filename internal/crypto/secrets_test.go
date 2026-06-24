package crypto

import "testing"

func TestEncryptDecrypt(t *testing.T) {
	service := New("test-secret")
	ciphertext, err := service.Encrypt([]byte("plain secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ciphertext == "plain secret" || ciphertext == "" {
		t.Fatalf("ciphertext should be non-empty and different")
	}
	plain, err := service.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(plain) != "plain secret" {
		t.Fatalf("plain mismatch: %q", plain)
	}
}

func TestFingerprint(t *testing.T) {
	if got := Fingerprint("secret-value"); len(got) != 12 {
		t.Fatalf("expected 12 char fingerprint, got %q", got)
	}
	if Fingerprint("") != "" {
		t.Fatalf("empty fingerprint should be empty")
	}
}
