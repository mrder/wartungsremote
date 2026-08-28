package platform

import "testing"

func hasAny(s, charset string) bool {
	for _, c := range s {
		for _, want := range charset {
			if c == want {
				return true
			}
		}
	}
	return false
}

func TestGenerateSupportPassword(t *testing.T) {
	for i := 0; i < 50; i++ {
		pw, err := GenerateSupportPassword(14)
		if err != nil {
			t.Fatal(err)
		}
		if len(pw) != 14 {
			t.Fatalf("expected length 14, got %d (%q)", len(pw), pw)
		}
		if !hasAny(pw, pwUpper) {
			t.Fatalf("password %q missing an uppercase character", pw)
		}
		if !hasAny(pw, pwLower) {
			t.Fatalf("password %q missing a lowercase character", pw)
		}
		if !hasAny(pw, pwDigits) {
			t.Fatalf("password %q missing a digit", pw)
		}
		if !hasAny(pw, pwSymbol) {
			t.Fatalf("password %q missing a symbol", pw)
		}
	}
}

func TestGenerateSupportPasswordNeverExceedsWindowsNetUserThreshold(t *testing.T) {
	// net user shows an interactive, unanswerable prompt for passwords
	// over 14 characters (found live) — this is the one length that must
	// never regress.
	pw, err := GenerateSupportPassword(14)
	if err != nil {
		t.Fatal(err)
	}
	if len(pw) > 14 {
		t.Fatalf("password length %d exceeds the net-user-safe threshold of 14", len(pw))
	}
}
