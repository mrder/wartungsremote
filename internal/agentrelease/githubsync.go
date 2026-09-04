package agentrelease

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName    string    `json:"tag_name"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

// assetNamePattern matches the wr-agent-<os>-<arch>[.exe] binary naming
// convention already used by scripts/quickinstall-agent-{windows,linux}.
// It deliberately does not match "<name>.sha256" or "<name>.sig" sidecar
// assets, which are looked up separately per binary asset found.
var assetNamePattern = regexp.MustCompile(`^wr-agent-([a-z0-9]+)-([a-z0-9]+)(\.exe)?$`)

// GitHubSyncer imports agent-* GitHub Releases as agent_versions rows so an
// admin doesn't have to hand-copy version/hash/signature into the dashboard
// form after every release. GitHub is only ever a transport here: every
// import still goes through Repo.Create's Ed25519 signature verification
// against the same trusted offline key the manual form uses, so a release
// whose .sig sidecar doesn't verify is rejected exactly as it would be from
// the form — GitHub itself is never trusted to vouch for an artifact.
//
// Expects, per binary asset (e.g. wr-agent-windows-amd64.exe), two sidecar
// assets in the same release: "<name>.sha256" (plain lowercase hex, no
// filename suffix) and "<name>.sig" (base64) — see
// cmd/wr-release-sign's -sign output, which now writes both alongside its
// existing stdout summary.
type GitHubSyncer struct {
	DB            *Repo
	GitHubRepo    string // "owner/name"
	Token         string // optional; raises the API rate limit
	TrustedPubKey ed25519.PublicKey
	HTTPClient    *http.Client
	// APIBaseURL overrides https://api.github.com for tests; production
	// code should leave it empty.
	APIBaseURL string
}

func NewGitHubSyncer(db *Repo, githubRepo, token string, trustedPubKey ed25519.PublicKey) *GitHubSyncer {
	return &GitHubSyncer{
		DB: db, GitHubRepo: githubRepo, Token: token, TrustedPubKey: trustedPubKey,
		HTTPClient: &http.Client{Timeout: 20 * time.Second},
	}
}

type SyncResult struct {
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors"`
}

// SyncOnce fetches the repo's GitHub Releases, imports any agent-* release
// asset not already present (matched on version+os+architecture), and
// returns a summary. Assets missing a sidecar, or whose sidecar fails to
// verify, are reported in Errors rather than aborting the whole sync.
func (s *GitHubSyncer) SyncOnce(ctx context.Context) (SyncResult, error) {
	var result SyncResult

	releases, err := s.fetchReleases(ctx)
	if err != nil {
		return result, err
	}
	existing, err := s.DB.List(ctx)
	if err != nil {
		return result, fmt.Errorf("agentrelease: list existing releases: %w", err)
	}
	seen := make(map[string]bool, len(existing))
	for _, e := range existing {
		seen[e.Version+"|"+e.OSFamily+"|"+e.Architecture] = true
	}

	for _, gr := range releases {
		if !strings.HasPrefix(gr.TagName, "agent-") {
			continue
		}
		version := strings.TrimPrefix(strings.TrimPrefix(gr.TagName, "agent-"), "v")
		channel := "stable"
		if gr.Prerelease {
			channel = "beta"
		}
		assetsByName := make(map[string]ghAsset, len(gr.Assets))
		for _, a := range gr.Assets {
			assetsByName[a.Name] = a
		}

		for _, a := range gr.Assets {
			m := assetNamePattern.FindStringSubmatch(a.Name)
			if m == nil {
				continue
			}
			osFamily, arch := m[1], m[2]
			key := version + "|" + osFamily + "|" + arch
			if seen[key] {
				result.Skipped++
				continue
			}

			sumAsset, ok := assetsByName[a.Name+".sha256"]
			if !ok {
				result.Errors = append(result.Errors, fmt.Sprintf("%s (%s): missing .sha256 sidecar asset", a.Name, gr.TagName))
				continue
			}
			sigAsset, ok := assetsByName[a.Name+".sig"]
			if !ok {
				result.Errors = append(result.Errors, fmt.Sprintf("%s (%s): missing .sig sidecar asset", a.Name, gr.TagName))
				continue
			}
			sumHex, err := s.fetchText(ctx, sumAsset.BrowserDownloadURL)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: fetch .sha256: %v", a.Name, err))
				continue
			}
			sigB64, err := s.fetchText(ctx, sigAsset.BrowserDownloadURL)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: fetch .sig: %v", a.Name, err))
				continue
			}

			if _, err := s.DB.Create(ctx, Release{
				Version: version, OSFamily: osFamily, Architecture: arch, Channel: channel,
				ArtifactURL: a.BrowserDownloadURL, ArtifactSHA256Hex: sumHex, SignatureBase64: sigB64,
			}, s.TrustedPubKey); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", a.Name, err))
				continue
			}
			seen[key] = true
			result.Imported++
		}
	}
	return result, nil
}

// RunGitHubSyncSweeper periodically calls SyncOnce until ctx is cancelled,
// matching the sweeper pattern used elsewhere (e.g. alerting.RunSweeper). A
// failed sync is logged and retried on the next tick rather than stopping
// the sweeper — a transient GitHub API hiccup shouldn't require a server
// restart to recover from.
func RunGitHubSyncSweeper(ctx context.Context, syncer *GitHubSyncer, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result, err := syncer.SyncOnce(ctx)
			if err != nil {
				slog.Error("github release sync failed", "error", err)
				continue
			}
			if result.Imported > 0 || len(result.Errors) > 0 {
				slog.Info("github release sync completed", "imported", result.Imported, "skipped", result.Skipped, "errors", result.Errors)
			}
		}
	}
}

func (s *GitHubSyncer) fetchReleases(ctx context.Context) ([]ghRelease, error) {
	base := s.APIBaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	url := fmt.Sprintf("%s/repos/%s/releases", base, s.GitHubRepo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("agentrelease: fetch github releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("agentrelease: github api returned %d: %s", resp.StatusCode, string(body))
	}
	var releases []ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 5<<20)).Decode(&releases); err != nil {
		return nil, fmt.Errorf("agentrelease: decode github releases: %w", err)
	}
	return releases, nil
}

func (s *GitHubSyncer) fetchText(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}
