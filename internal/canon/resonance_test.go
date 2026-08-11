package canon

import "testing"

func TestResonanceLevelForXP(t *testing.T) {
	cases := []struct {
		xp   int
		want int
	}{
		{0, 1}, {99, 1}, {100, 2}, {349, 2}, {350, 3}, {849, 3},
		{850, 4}, {1749, 4}, {1750, 5}, {99999, 5},
	}
	for _, c := range cases {
		if got := ResonanceLevelForXP(c.xp); got != c.want {
			t.Errorf("ResonanceLevelForXP(%d) = %d, want %d", c.xp, got, c.want)
		}
	}
}

func TestResonanceControlCap(t *testing.T) {
	want := map[int]int{1: 2, 2: 3, 3: 4, 4: 5, 5: 6}
	for lvl, cap := range want {
		if got := ResonanceControlCap(lvl); got != cap {
			t.Errorf("ResonanceControlCap(%d) = %d, want %d", lvl, got, cap)
		}
	}
	// Out-of-range clamps.
	if ResonanceControlCap(0) != 2 || ResonanceControlCap(99) != 6 {
		t.Error("control cap should clamp to RL1/RL5 bounds")
	}
}

func TestResonanceXPForNextLevel(t *testing.T) {
	if ResonanceXPForNextLevel(1) != 100 {
		t.Errorf("RL1 next = %d, want 100", ResonanceXPForNextLevel(1))
	}
	if ResonanceXPForNextLevel(ResonanceMaxLevel) != -1 {
		t.Error("max level should return -1 for next-XP")
	}
}
