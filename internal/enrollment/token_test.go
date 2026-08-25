package enrollment

import "testing"

func TestGenerateTokenHashRoundTrip(t *testing.T) {
	token, hash, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	if len(hash) != 32 { // SHA-256
		t.Fatalf("expected 32-byte hash, got %d", len(hash))
	}

	rehashed, err := hashToken(token)
	if err != nil {
		t.Fatalf("hashToken: %v", err)
	}
	if string(rehashed) != string(hash) {
		t.Fatal("expected hashToken(token) to reproduce the hash computed at generation time")
	}
}

func TestGenerateTokenIsHighEntropyAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		token, _, err := generateToken()
		if err != nil {
			t.Fatalf("generateToken: %v", err)
		}
		if seen[token] {
			t.Fatalf("duplicate token generated: %s", token)
		}
		seen[token] = true
	}
}

func TestHashTokenRejectsWrongPrefix(t *testing.T) {
	if _, err := hashToken("not_a_valid_token"); err == nil {
		t.Fatal("expected hashToken to reject a token without the wr_enroll_ prefix")
	}
}

func TestHashTokenRejectsMalformedSuffix(t *testing.T) {
	if _, err := hashToken("wr_enroll_not-valid-base64!!!"); err == nil {
		t.Fatal("expected hashToken to reject invalid base64 suffix")
	}
}

func TestDifferentTokensHashDifferently(t *testing.T) {
	t1, h1, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	t2, h2, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	if t1 == t2 {
		t.Fatal("expected distinct tokens")
	}
	if string(h1) == string(h2) {
		t.Fatal("expected distinct hashes for distinct tokens")
	}
}
