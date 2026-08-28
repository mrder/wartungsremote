package notify

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	r := &TelegramRepo{key: key}

	plaintext := []byte("123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11")
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
