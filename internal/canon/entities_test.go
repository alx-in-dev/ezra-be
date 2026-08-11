package canon

import "testing"

func TestStarterRoster(t *testing.T) {
	cases := []struct {
		pool int
		want []string
	}{
		{0, []string{EntityAssault}},  // guaranteed floor
		{5, []string{EntityAssault}},  // below first threshold → still guaranteed
		{6, []string{EntityAssault}},  // first unlock
		{10, []string{EntityAssault, EntityScout}},
		{14, []string{EntityAssault, EntityScout, EntityDistortion}},
		{18, []string{EntityAssault, EntityScout, EntityDistortion, EntityAbsorber}},
		{22, []string{EntityAssault, EntityScout, EntityDistortion, EntityAbsorber, EntityAssault}},
		{99, []string{EntityAssault, EntityScout, EntityDistortion, EntityAbsorber, EntityAssault}},
	}
	for _, c := range cases {
		got := StarterRoster(c.pool)
		if len(got) != len(c.want) {
			t.Fatalf("pool %d: got %v, want %v", c.pool, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("pool %d: got %v, want %v", c.pool, got, c.want)
			}
		}
	}
}

func TestEntitySpecsCanon(t *testing.T) {
	// Cap weights: absorber costs 2, others 1 (symbiont_roster.md §"Control cap weights").
	if EntityCapWeight(EntityAbsorber) != 2 {
		t.Fatalf("absorber cap weight = %d, want 2", EntityCapWeight(EntityAbsorber))
	}
	for _, a := range []string{EntityAssault, EntityDistortion, EntityScout} {
		if EntityCapWeight(a) != 1 {
			t.Fatalf("%s cap weight = %d, want 1", a, EntityCapWeight(a))
		}
	}
	// RL unlocks: assault+scout RL1, distortion RL2, absorber RL3.
	wantUnlock := map[string]int{EntityAssault: 1, EntityScout: 1, EntityDistortion: 2, EntityAbsorber: 3}
	for a, lvl := range wantUnlock {
		if EntityUnlockLevel(a) != lvl {
			t.Fatalf("%s unlock = %d, want %d", a, EntityUnlockLevel(a), lvl)
		}
	}
	// Target kinds: assault/distortion → tower; scout/absorber → rift.
	for _, a := range []string{EntityAssault, EntityDistortion} {
		if EntityTargetKind(a) != EntityTargetTower {
			t.Fatalf("%s target = %s, want tower", a, EntityTargetKind(a))
		}
	}
	for _, a := range []string{EntityScout, EntityAbsorber} {
		if EntityTargetKind(a) != EntityTargetRift {
			t.Fatalf("%s target = %s, want rift", a, EntityTargetKind(a))
		}
	}
}

func TestAttunement(t *testing.T) {
	// Rank by cumulative uses: II at 10, III at 30.
	ranks := map[int]int{0: 1, 9: 1, 10: 2, 29: 2, 30: 3, 100: 3}
	for uses, want := range ranks {
		if got := AttunementRankForUses(uses); got != want {
			t.Fatalf("AttunementRankForUses(%d) = %d, want %d", uses, got, want)
		}
	}
	// Next-rank thresholds for client readouts.
	if AttunementUsesForNextRank(1) != 10 || AttunementUsesForNextRank(2) != 30 || AttunementUsesForNextRank(3) != -1 {
		t.Fatal("AttunementUsesForNextRank thresholds wrong")
	}
	// Effect multipliers: I ×1.0, II +12%, III +28%.
	if AttunementEffectMultiplier(1) != 1.0 || AttunementEffectMultiplier(2) != 1.12 || AttunementEffectMultiplier(3) != 1.28 {
		t.Fatal("AttunementEffectMultiplier wrong")
	}
	// Rank III perk: −33% recovery; I/II unaffected.
	if AttunementRecoveryFactor(1) != 1.0 || AttunementRecoveryFactor(2) != 1.0 || AttunementRecoveryFactor(3) != 0.67 {
		t.Fatal("AttunementRecoveryFactor wrong")
	}
}

func TestUnknownArchetype(t *testing.T) {
	if IsEntityArchetype("ghost") {
		t.Fatal("unknown archetype reported as valid")
	}
	if EntityUnlockLevel("ghost") <= ResonanceMaxLevel {
		t.Fatal("unknown archetype should never unlock")
	}
}
