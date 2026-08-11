package canon

import "testing"

func TestBeaconPressureDamagePerHour(t *testing.T) {
	cases := []struct {
		infection float64
		want      float64
	}{
		{0, 0},
		{50, 0},   // at/below threshold → safe (domed beacons sit here)
		{40, 0},   // below threshold
		{70, 10},  // (70-50)*0.5
		{100, 25}, // max grind
	}
	for _, c := range cases {
		if got := BeaconPressureDamagePerHour(c.infection); got != c.want {
			t.Errorf("BeaconPressureDamagePerHour(%v) = %v, want %v", c.infection, got, c.want)
		}
	}
}

func TestLinkCost(t *testing.T) {
	cases := []struct {
		lengthM float64
		want    int
	}{
		{50, 3},    // ceil(0.5)=1 *3
		{200, 6},   // 2*3
		{500, 15},  // 5*3
		{1000, 30}, // 10*3
	}
	for _, c := range cases {
		if got := LinkCost(c.lengthM); got != c.want {
			t.Errorf("LinkCost(%v) = %d, want %d", c.lengthM, got, c.want)
		}
	}
}

func TestCoreEcap(t *testing.T) {
	if CoreEcap(1) != 100 {
		t.Errorf("CoreEcap(1) = %d, want 100", CoreEcap(1))
	}
	if CoreEcap(99) != 100 { // unknown level clamps to L1
		t.Errorf("CoreEcap(99) = %d, want 100 (clamped)", CoreEcap(99))
	}
}

func TestBeaconAreaCapacity(t *testing.T) {
	cases := []struct {
		buildingCells int
		want          int
	}{
		{0, BeaconCapacityMin},   // empty area → floor
		{1, BeaconCapacityMin},   // 3 + 1 = 4 > min(3) → 4
		{-5, BeaconCapacityMin},  // never below floor
		{100, BeaconCapacityHardCap}, // dense → clamped to hard cap
	}
	// case {1} actually computes 4; assert via formula not the floor literal.
	if got := BeaconAreaCapacity(1); got != 4 {
		t.Fatalf("BeaconAreaCapacity(1) = %d, want 4", got)
	}
	for _, c := range cases {
		got := BeaconAreaCapacity(c.buildingCells)
		if c.buildingCells == 1 {
			continue // handled above
		}
		if got != c.want {
			t.Fatalf("BeaconAreaCapacity(%d) = %d, want %d", c.buildingCells, got, c.want)
		}
		if got < BeaconCapacityMin || got > BeaconCapacityHardCap {
			t.Fatalf("BeaconAreaCapacity(%d) = %d out of [%d,%d]", c.buildingCells, got, BeaconCapacityMin, BeaconCapacityHardCap)
		}
	}
}

func TestBeaconAreaCapacityWith_StationBonus(t *testing.T) {
	base := BeaconAreaCapacity(2) // 3 + 2 = 5
	withBonus := BeaconAreaCapacityWith(2, 4)
	if withBonus != base+4 {
		t.Fatalf("station bonus: want %d, got %d", base+4, withBonus)
	}
	// Hard cap still bounds the total.
	if got := BeaconAreaCapacityWith(100, 100); got != BeaconCapacityHardCap {
		t.Fatalf("station bonus must not breach hard cap, got %d", got)
	}
}
