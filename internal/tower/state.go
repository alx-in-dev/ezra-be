package tower

import "github.com/ezra-game/server/internal/canon"

func ResolveState(overloadCount int) string {
	return canon.ResolveTowerState(overloadCount)
}
