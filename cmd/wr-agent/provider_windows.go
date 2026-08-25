//go:build windows

package main

import (
	"wartungsremote/internal/platform"
	"wartungsremote/internal/platform/windows"
)

func newProvider(agentVersion string) platform.Provider {
	return windows.New(agentVersion)
}
