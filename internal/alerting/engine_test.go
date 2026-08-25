package alerting

import (
	"testing"

	"github.com/google/uuid"

	"wartungsremote/internal/device"
)

func TestScopedDevices(t *testing.T) {
	customerA := uuid.New()
	customerB := uuid.New()
	groupA := uuid.New()

	dev1 := device.Device{ID: uuid.New(), CustomerID: &customerA}
	dev2 := device.Device{ID: uuid.New(), CustomerID: &customerB, GroupID: &groupA}
	dev3 := device.Device{ID: uuid.New()}
	all := []device.Device{dev1, dev2, dev3}

	t.Run("global matches every device", func(t *testing.T) {
		got := scopedDevices(all, Rule{ScopeType: ScopeGlobal})
		if len(got) != 3 {
			t.Fatalf("expected 3 devices, got %d", len(got))
		}
	})

	t.Run("customer scope filters by customer id", func(t *testing.T) {
		got := scopedDevices(all, Rule{ScopeType: ScopeCustomer, ScopeID: &customerA})
		if len(got) != 1 || got[0].ID != dev1.ID {
			t.Fatalf("expected only dev1, got %+v", got)
		}
	})

	t.Run("group scope filters by group id", func(t *testing.T) {
		got := scopedDevices(all, Rule{ScopeType: ScopeGroup, ScopeID: &groupA})
		if len(got) != 1 || got[0].ID != dev2.ID {
			t.Fatalf("expected only dev2, got %+v", got)
		}
	})

	t.Run("device scope matches only that device", func(t *testing.T) {
		got := scopedDevices(all, Rule{ScopeType: ScopeDevice, ScopeID: &dev3.ID})
		if len(got) != 1 || got[0].ID != dev3.ID {
			t.Fatalf("expected only dev3, got %+v", got)
		}
	})

	t.Run("missing scope id yields no matches for non-global scopes", func(t *testing.T) {
		if got := scopedDevices(all, Rule{ScopeType: ScopeCustomer}); len(got) != 0 {
			t.Fatalf("expected no devices, got %+v", got)
		}
	})
}
