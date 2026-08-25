//go:build linux

package main

import (
	"wartungsremote/internal/platform"
	"wartungsremote/internal/platform/linux"
)

func newProvider(agentVersion string) platform.Provider {
	return linux.New(agentVersion)
}
