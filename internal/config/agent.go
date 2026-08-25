package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AgentPolicy is the local, agent-enforced policy per docs/CONFIGURATION.md §4.
// A remote action requires BOTH the server permission AND this local policy
// to allow it; if either denies, the action is denied.
type AgentPolicy struct {
	Terminal         bool `yaml:"terminal"`
	SSHTunnel        bool `yaml:"ssh_tunnel"`
	RDPTunnel        bool `yaml:"rdp_tunnel"`
	FilesRead        bool `yaml:"files_read"`
	FilesWrite       bool `yaml:"files_write"`
	ServiceControl   bool `yaml:"service_control"`
	ProcessTerminate bool `yaml:"process_terminate"`
	PowerControl     bool `yaml:"power_control"`
	SelfUpdate       bool `yaml:"self_update"`
}

// AgentConfig is the wr-agent configuration (non-sensitive; credentials are
// stored separately, see internal/agentcore/identity.go).
type AgentConfig struct {
	ServerURL      string      `yaml:"server_url"`
	UpdateChannel  string      `yaml:"update_channel"`
	LogLevel       string      `yaml:"log_level"`
	Policy         AgentPolicy `yaml:"policy"`
	ConfigDir      string      `yaml:"-"`
	DataDir        string      `yaml:"-"`
	LogDir         string      `yaml:"-"`
}

// DefaultAgent returns safe defaults; every policy flag defaults to allowed
// so that the effective permission is governed by the server side unless the
// installer/administrator explicitly restricts it locally.
func DefaultAgent() AgentConfig {
	return AgentConfig{
		UpdateChannel: "stable",
		LogLevel:      "info",
		Policy: AgentPolicy{
			Terminal:         true,
			SSHTunnel:        true,
			RDPTunnel:        true,
			FilesRead:        true,
			FilesWrite:       true,
			ServiceControl:   true,
			ProcessTerminate: true,
			PowerControl:     true,
			SelfUpdate:       true,
		},
	}
}

// LoadAgent reads agent.yaml from path, overlaying defaults.
func LoadAgent(path string) (AgentConfig, error) {
	c := DefaultAgent()
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, fmt.Errorf("config: agent config not found at %s: %w", path, err)
		}
		return c, fmt.Errorf("config: read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(body, &c); err != nil {
		return c, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if c.ServerURL == "" {
		return c, fmt.Errorf("config: server_url is required")
	}
	return c, nil
}
