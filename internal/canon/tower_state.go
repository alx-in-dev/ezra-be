package canon

// ResolveTowerState reports a tower's crowding state against the flat legacy
// limit. Kept for callers without a place-aware capacity (map summaries, pet).
func ResolveTowerState(overloadCount int) string {
	return ResolveTowerStateCap(overloadCount, TowerOverloadMaxCount)
}

// ResolveTowerStateCap reports the crowding state against a place-aware capacity
// (energy pillar): overloaded once the area's beacon count reaches its grid
// capacity. capacity <= 0 falls back to the flat limit.
func ResolveTowerStateCap(overloadCount, capacity int) string {
	if capacity <= 0 {
		capacity = TowerOverloadMaxCount
	}
	if overloadCount >= capacity {
		return TowerStateOverloaded
	}
	return TowerStateActive
}
