//go:build linux

package linux

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"wartungsremote/internal/platform"
)

// cleanPath enforces the minimum path hygiene required regardless of
// whether the deployment restricts a root: reject empty/relative paths and
// normalize `.`/`..` segments so a caller can't rely on OS-specific
// quirks. V1 may permit system-wide access for privileged administration
// (docs/AGENT.md §13), but every path must still be explicit and canonical
// — never assembled via string concatenation with untrusted fragments.
func cleanPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("linux: empty path")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("linux: path must be absolute: %q", path)
	}
	return filepath.Clean(path), nil
}

func (p *Provider) ListDir(ctx context.Context, path string) ([]platform.FileEntry, error) {
	clean, err := cleanPath(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		return nil, fmt.Errorf("linux: list dir: %w", err)
	}
	out := make([]platform.FileEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, platform.FileEntry{
			Name:    e.Name(),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().UnixMilli(),
		})
	}
	return out, nil
}

func (p *Provider) Mkdir(ctx context.Context, path string) error {
	clean, err := cleanPath(path)
	if err != nil {
		return err
	}
	if err := os.Mkdir(clean, 0o755); err != nil {
		return fmt.Errorf("linux: mkdir: %w", err)
	}
	return nil
}

func (p *Provider) Rename(ctx context.Context, from, to string) error {
	cleanFrom, err := cleanPath(from)
	if err != nil {
		return err
	}
	cleanTo, err := cleanPath(to)
	if err != nil {
		return err
	}
	if err := os.Rename(cleanFrom, cleanTo); err != nil {
		return fmt.Errorf("linux: rename: %w", err)
	}
	return nil
}

func (p *Provider) Delete(ctx context.Context, path string) error {
	clean, err := cleanPath(path)
	if err != nil {
		return err
	}
	if err := os.Remove(clean); err != nil {
		return fmt.Errorf("linux: delete: %w", err)
	}
	return nil
}

func (p *Provider) ReadFile(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	clean, err := cleanPath(path)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(clean)
	if err != nil {
		return nil, 0, fmt.Errorf("linux: open file: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, fmt.Errorf("linux: stat file: %w", err)
	}
	if info.IsDir() {
		f.Close()
		return nil, 0, fmt.Errorf("linux: %q is a directory", clean)
	}
	return f, info.Size(), nil
}

// WriteFile writes to a temporary file in the same directory, then renames
// atomically into place, per docs/SPECIFICATION.md §17 ("Upload zunächst in
// temporäre Datei, danach ... atomare Umbenennung").
func (p *Provider) WriteFile(ctx context.Context, path string) (io.WriteCloser, error) {
	clean, err := cleanPath(path)
	if err != nil {
		return nil, err
	}
	tmp := clean + ".wr-upload.tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("linux: create temp file: %w", err)
	}
	return &atomicWriteCloser{f: f, tmpPath: tmp, finalPath: clean}, nil
}

type atomicWriteCloser struct {
	f         *os.File
	tmpPath   string
	finalPath string
}

func (w *atomicWriteCloser) Write(p []byte) (int, error) { return w.f.Write(p) }

func (w *atomicWriteCloser) Close() error {
	if err := w.f.Close(); err != nil {
		os.Remove(w.tmpPath)
		return fmt.Errorf("linux: close temp file: %w", err)
	}
	if err := os.Rename(w.tmpPath, w.finalPath); err != nil {
		os.Remove(w.tmpPath)
		return fmt.Errorf("linux: finalize upload: %w", err)
	}
	return nil
}
