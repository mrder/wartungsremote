// Requires a real PostgreSQL instance and is skipped unless
// WR_TEST_DATABASE_URL is set (see tests/integration_test.go for the same
// convention used elsewhere in this repo).
package agentrelease

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"wartungsremote/internal/db"
	"wartungsremote/migrations"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("WR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("WR_TEST_DATABASE_URL not set; skipping database integration test")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// newMockGitHub returns a syncer wired to an httptest server that serves one
// agent-<tag> release with a single wr-agent-<os>-<arch> asset plus its
// .sha256/.sig sidecars, signed with signKey. Passing a different pubKey to
// the syncer than the one signKey corresponds to lets a test exercise the
// rejection path.
func newMockGitHub(t *testing.T, repo *Repo, tag, assetName string, signKey ed25519.PrivateKey, trustedPub ed25519.PublicKey) *GitHubSyncer {
	t.Helper()
	sum := sha256.Sum256([]byte("irrelevant artifact bytes, only the hash is checked"))
	sumHex := hex.EncodeToString(sum[:])
	sig := ed25519.Sign(signKey, sum[:])
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/widget/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]ghRelease{
			{
				TagName:    tag,
				Prerelease: false,
				Assets: []ghAsset{
					{Name: assetName, BrowserDownloadURL: srv.URL + "/asset-bin"},
					{Name: assetName + ".sha256", BrowserDownloadURL: srv.URL + "/asset-sha256"},
					{Name: assetName + ".sig", BrowserDownloadURL: srv.URL + "/asset-sig"},
				},
			},
		})
	})
	mux.HandleFunc("/asset-sha256", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(sumHex)) })
	mux.HandleFunc("/asset-sig", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(sigB64)) })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	syncer := NewGitHubSyncer(repo, "acme/widget", "", trustedPub)
	syncer.APIBaseURL = srv.URL
	return syncer
}

func TestGitHubSyncerImportsValidReleaseAndSkipsOnRerun(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewRepo(pool)

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	// A unique version per test run — this repo persists across runs (it's
	// exercised against a real, non-transactional database, per testPool),
	// so a fixed version would collide with a row a previous run left behind
	// and be (correctly) skipped as already-imported instead of exercising
	// the fresh-import path this test is for.
	version := fmt.Sprintf("0.0.%d", time.Now().UnixNano())
	syncer := newMockGitHub(t, repo, "agent-v"+version, "wr-agent-linux-amd64", priv, pub)

	result, err := syncer.SyncOnce(ctx)
	if err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if result.Imported != 1 {
		t.Fatalf("Imported = %d, want 1", result.Imported)
	}

	releases, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range releases {
		if r.Version == version && r.OSFamily == "linux" && r.Architecture == "amd64" {
			found = true
		}
	}
	if !found {
		t.Fatal("imported release not found in repo.List()")
	}

	// Re-running the same sync must not re-import or duplicate-insert —
	// this is what makes it safe to run on a timer.
	result2, err := syncer.SyncOnce(ctx)
	if err != nil {
		t.Fatalf("second SyncOnce: %v", err)
	}
	if result2.Imported != 0 || result2.Skipped != 1 {
		t.Fatalf("second run = %+v, want Imported=0 Skipped=1", result2)
	}
}

func TestGitHubSyncerRejectsWrongSignature(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewRepo(pool)

	trustedPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, wrongPriv, err := ed25519.GenerateKey(rand.Reader) // signs with a DIFFERENT key than trustedPub
	if err != nil {
		t.Fatal(err)
	}
	syncer := newMockGitHub(t, repo, "agent-v9.9.8", "wr-agent-windows-amd64.exe", wrongPriv, trustedPub)

	result, err := syncer.SyncOnce(ctx)
	if err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
	if result.Imported != 0 {
		t.Fatalf("Imported = %d, want 0 for a bad signature", result.Imported)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("Errors = %v, want exactly one signature-verification failure", result.Errors)
	}
}
