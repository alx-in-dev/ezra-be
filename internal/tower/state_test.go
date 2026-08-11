package tower

import (
	"testing"

	"github.com/ezra-game/server/internal/canon"
)

func TestResolveState_UsesOverloadThreshold(t *testing.T) {
	if got := ResolveState(canon.TowerOverloadMaxCount - 1); got != canon.TowerStateActive {
		t.Fatalf("expected active below threshold, got %s", got)
	}
	if got := ResolveState(canon.TowerOverloadMaxCount); got != canon.TowerStateOverloaded {
		t.Fatalf("expected overloaded at threshold, got %s", got)
	}
}
