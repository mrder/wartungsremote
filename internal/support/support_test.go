package support

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	r := &Repo{key: key}

	plaintext := []byte("wr_super_secret_password_123!")
	ciphertext, nonce, err := r.encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if string(ciphertext) == string(plaintext) {
		t.Fatal("ciphertext must not equal plaintext")
	}

	got, err := r.decrypt(ciphertext, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("got %q, want %q", got, plaintext)
	}
}

func TestDecryptFailsWithWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	for i := range key2 {
		key2[i] = byte(i + 1)
	}
	r1 := &Repo{key: key1}
	r2 := &Repo{key: key2}

	ciphertext, nonce, err := r1.encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r2.decrypt(ciphertext, nonce); err == nil {
		t.Fatal("expected decryption with the wrong key to fail")
	}
}
