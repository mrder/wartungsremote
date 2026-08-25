package agentcore

import (
	"os"
	"path/filepath"
	"runtime"
)

// DefaultPaths returns the documented default configuration, data and log
// directories for the current OS, per docs/AGENT.md §2.
type Paths struct {
	ConfigFile string
	DataDir    string
	LogDir     string
}

func DefaultPaths() Paths {
	if runtime.GOOS == "windows" {
		base := os.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		root := filepath.Join(base, "WartungsRemote")
		return Paths{
			ConfigFile: filepath.Join(root, "config", "agent.yaml"),
			DataDir:    filepath.Join(root, "data"),
			LogDir:     filepath.Join(root, "logs"),
		}
	}
	return Paths{
		ConfigFile: "/etc/wartungsremote/agent.yaml",
		DataDir:    "/var/lib/wartungsremote",
		LogDir:     "/var/log/wartungsremote",
	}
}
