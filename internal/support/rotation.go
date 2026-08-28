package support

import (
	"context"
	"log/slog"
	"time"

	"wartungsremote/internal/controlhub"
	"wartungsremote/internal/protocol"
)

// RotationSource supplies the current effective rotation interval on every
// sweep tick, so a runtime change (dashboard Settings page) takes effect
// without a server restart — same pattern as device.RetentionSource.
type RotationSource interface {
	SupportCredentialRotationDays(ctx context.Context) (int, error)
}

// RunRotationSweeper periodically rotates the remote-support account
// password on any device whose credential is older than the configured
// interval. Disabled (the default) until an interval is explicitly set —
// see docs/AGENT.md "Remote-support account". A device that's offline when
// its rotation comes due is simply retried on the next tick; nothing is
// lost or skipped permanently.
func RunRotationSweeper(ctx context.Context, repo *Repo, settings RotationSource, hub *controlhub.Hub, checkInterval time.Duration) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			days, err := settings.SupportCredentialRotationDays(ctx)
			if err != nil {
				slog.Error("support credential rotation: failed to resolve settings, skipping this tick", "error", err)
				continue
			}
			if days <= 0 {
				continue // disabled
			}
			deviceIDs, err := repo.ListDueForRotation(ctx, time.Duration(days)*24*time.Hour)
			if err != nil {
				slog.Error("support credential rotation sweep failed", "error", err)
				continue
			}
			for _, id := range deviceIDs {
				if !hub.IsOnline(id) {
					continue
				}
				if err := hub.SendMessage(ctx, id, protocol.TypeDeviceCommand, protocol.DeviceCommandPayload{
					CommandType: protocol.CmdRotateSupportCredential,
				}); err != nil {
					slog.Error("failed to send scheduled support credential rotation", "device_id", id, "error", err)
				}
			}
		}
	}
}
