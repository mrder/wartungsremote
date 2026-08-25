// wr-release-sign is an offline tool for generating an agent release
// signing keypair and signing release artifacts (docs/AGENT.md §15). It is
// never bundled into, invoked by, or reachable from wr-core or wr-agent —
// the private key it produces must never touch an online system, per
// docs/API.md §18 ("Server erzeugt nicht ad hoc eine vertrauenswürdige
// Signatur mit einem online verfügbaren Produktionskey").
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
)

func main() {
	genKey := flag.Bool("genkey", false, "generate a new Ed25519 release signing keypair")
	keyOut := flag.String("out", "release", "output file prefix for -genkey (writes <prefix>.key and <prefix>.pub, each 32 raw bytes)")
	signPath := flag.String("sign", "", "path to an artifact file to sign")
	keyFile := flag.String("key", "", "path to the Ed25519 private key file (32 raw bytes, from -genkey's <prefix>.key) for -sign")
	flag.Parse()

	switch {
	case *genKey:
		if err := runGenKey(*keyOut); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case *signPath != "":
		if err := runSign(*signPath, *keyFile); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage:")
		fmt.Fprintln(os.Stderr, "  wr-release-sign -genkey [-out prefix]")
		fmt.Fprintln(os.Stderr, "  wr-release-sign -sign <artifact> -key <private-key-file>")
		os.Exit(2)
	}
}

func runGenKey(outPrefix string) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate keypair: %w", err)
	}
	// Ed25519 private keys from this package are the 64-byte "seed || pub"
	// form; writing it raw keeps -sign's read path trivial. Guard the file
	// mode so the private key isn't world-readable at rest.
	if err := os.WriteFile(outPrefix+".key", priv, 0o600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}
	if err := os.WriteFile(outPrefix+".pub", pub, 0o644); err != nil {
		return fmt.Errorf("write public key: %w", err)
	}
	fmt.Printf("wrote %s.key (private, keep offline) and %s.pub (public, install as WR_RELEASE_PUBLIC_KEY_FILE on the server)\n", outPrefix, outPrefix)
	return nil
}

func runSign(artifactPath, keyPath string) error {
	if keyPath == "" {
		return fmt.Errorf("-key is required with -sign")
	}
	priv, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read private key: %w", err)
	}
	if len(priv) != ed25519.PrivateKeySize {
		return fmt.Errorf("private key file must be exactly %d raw bytes, got %d", ed25519.PrivateKeySize, len(priv))
	}
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return fmt.Errorf("read artifact: %w", err)
	}
	sum := sha256.Sum256(data)
	sig := ed25519.Sign(ed25519.PrivateKey(priv), sum[:])

	fmt.Printf("artifact_sha256: %s\n", hex.EncodeToString(sum[:]))
	fmt.Printf("signature:       %s\n", base64.StdEncoding.EncodeToString(sig))
	return nil
}
