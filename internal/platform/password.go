package platform

import (
	"crypto/rand"
	"math/big"
)

// Ambiguous-looking characters (0/O, 1/l/I) excluded — this password is
// never meant to be hand-typed, but excluding them costs nothing and helps
// if someone ever has to read it off a screen.
const (
	pwUpper  = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	pwLower  = "abcdefghijkmnopqrstuvwxyz"
	pwDigits = "23456789"
	pwSymbol = "!@#$%^&*-_"
)

// GenerateSupportPassword produces a random password guaranteed to contain
// at least one character from each of upper/lower/digit/symbol — Windows'
// default local-account complexity policy requires 3 of those 4 classes,
// and this reliably clears it rather than relying on chance.
//
// length is capped in practice at 14 by both callers: `net user` shows an
// interactive "this password won't work with pre-Windows-2000 clients,
// continue? (Y/N)" prompt for anything longer, which hangs forever since
// nothing feeds it stdin (found live) — no such constraint on Linux, but
// 14 random characters from this alphabet is already ~80 bits of entropy,
// plenty for a local account password, so both platforms just use the
// same length rather than needing two code paths.
func GenerateSupportPassword(length int) (string, error) {
	all := pwUpper + pwLower + pwDigits + pwSymbol
	classes := []string{pwUpper, pwLower, pwDigits, pwSymbol}
	if length < len(classes) {
		length = len(classes)
	}

	buf := make([]byte, length)
	for i, class := range classes {
		c, err := randomChar(class)
		if err != nil {
			return "", err
		}
		buf[i] = c
	}
	for i := len(classes); i < length; i++ {
		c, err := randomChar(all)
		if err != nil {
			return "", err
		}
		buf[i] = c
	}

	// Fisher-Yates shuffle so the guaranteed classes aren't always in the
	// first few positions.
	for i := length - 1; i > 0; i-- {
		j, err := randomInt(i + 1)
		if err != nil {
			return "", err
		}
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf), nil
}

func randomChar(charset string) (byte, error) {
	i, err := randomInt(len(charset))
	if err != nil {
		return 0, err
	}
	return charset[i], nil
}

func randomInt(max int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}
