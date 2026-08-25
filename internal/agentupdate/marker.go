package agentupdate

import (
	"encoding/json"
	"os"
)

// Marker records an in-progress update across a process restart, so the
// next startup can tell "am I the freshly-updated binary?" and decide
// whether to commit (delete the backup) or roll back (restore it) — see
// docs/AGENT.md §15 steps 9-11.
type Marker struct {
	Version      string `json:"version"`
	BackupPath   string `json:"backup_path"`
	BootAttempts int    `json:"boot_attempts"`
}

// MaxBootAttempts is how many consecutive process starts with an
// uncommitted marker are tolerated before RestoreBackup is triggered. A
// start "commits" (deletes the marker) once the control channel connects
// successfully, so this only fires on a genuine crash loop.
const MaxBootAttempts = 3

func LoadMarker(path string) (Marker, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Marker{}, false, nil
		}
		return Marker{}, false, err
	}
	var m Marker
	if err := json.Unmarshal(data, &m); err != nil {
		return Marker{}, false, err
	}
	return m, true, nil
}

func SaveMarker(path string, m Marker) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func DeleteMarker(path string) error {
	err := os.Remove(path)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}
