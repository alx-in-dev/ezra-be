package canon

import "testing"

func TestRiftDifficultyScale(t *testing.T) {
	cases := []struct {
		level int
		want  float64
	}{
		{0, riftScaleFloor},  // guarded to level 1 → below reference → floor
		{1, riftScaleFloor},  // 1 + (1-8)*0.03 = 0.79 → clamped to 0.85
		{8, 1.0},             // reference level → unscaled
		{15, 1.21},           // 1 + 7*0.03
		{100, riftScaleCeil}, // far above → clamped to ceil
	}
	for _, c := range cases {
		got := RiftDifficultyScale(c.level)
		if diff := got - c.want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("RiftDifficultyScale(%d) = %v, want %v", c.level, got, c.want)
		}
	}
	// Monotonic non-decreasing in level.
	prev := 0.0
	for lvl := 1; lvl <= 40; lvl++ {
		f := RiftDifficultyScale(lvl)
		if f < prev {
			t.Fatalf("scale not monotonic at level %d: %v < %v", lvl, f, prev)
		}
		prev = f
	}
}

func TestCurrentReleaseStageV1_0Toggles(t *testing.T) {
	if CurrentReleaseStage != ReleaseStageV1_0 {
		t.Fatalf("expected current release stage to be v1.0, got %s", CurrentReleaseStage)
	}
	// Phase D unlocks at v1.0.
	if !IsFeatureAvailable(FeatureBattleOvercharge) {
		t.Fatalf("overcharge must be enabled at v1.0")
	}
	if !IsTowerLevelAvailable(4) || !IsTowerLevelAvailable(5) {
		t.Fatalf("tower levels 4/5 must be enabled at v1.0")
	}
	if !IsTowerLevelAvailable(3) {
		t.Fatalf("tower level 3 must stay enabled")
	}
	// tower_pressure is a v1.1 toggle — still gated.
	if IsFeatureAvailable(FeatureTowerPressure) {
		t.Fatalf("tower pressure must stay disabled until v1.1")
	}
}

func TestTowerLevelSpecsMatchPhaseZeroCanon(t *testing.T) {
	level3 := TowerLevelSpecs[3]
	if level3.HPMax != 220 || level3.EnergyCost != 400 || level3.MaterialCost != 80 {
		t.Fatalf("unexpected L3 spec: %+v", level3)
	}

	level5 := TowerLevelSpecs[5]
	if level5.HPMax != 450 || level5.EffectPerHour != 16.0 {
		t.Fatalf("unexpected L5 spec: %+v", level5)
	}
	if level5.EnergyPerHour != 16 || level5.MaterialsPerHour != 3 {
		t.Fatalf("unexpected L5 passive income: %+v", level5)
	}
}

func TestEveryBeaconYieldsMaterials(t *testing.T) {
	// C5: every powered beacon, including the entry levels, must trickle
	// materials so infrastructure-poor regions are not 🔩-starved.
	for lvl := 1; lvl <= 5; lvl++ {
		if got := TowerLevelSpecs[lvl].MaterialsPerHour; got < 1 {
			t.Errorf("L%d must yield >=1 material/hour, got %d", lvl, got)
		}
	}
}

func TestBattleTacticsMVPMatchesCanon(t *testing.T) {
	expected := []string{
		BattleTacticAttack,
		BattleTacticDefend,
		BattleTacticEnergy,
		BattleTacticRetreat,
	}
	if len(BattleTacticsMVP) != len(expected) {
		t.Fatalf("unexpected tactic count: %d", len(BattleTacticsMVP))
	}
	for _, tactic := range expected {
		if _, ok := BattleTacticsMVP[tactic]; !ok {
			t.Fatalf("missing tactic %s", tactic)
		}
	}
}

func TestInfectionBandMatchesCanonThresholds(t *testing.T) {
	cases := []struct {
		infection float64
		expected  string
	}{
		{0, InfectionBandSafe},
		{20, InfectionBandSafe},
		{21, InfectionBandAlert},
		{40, InfectionBandAlert},
		{41, InfectionBandDangerous},
		{60, InfectionBandDangerous},
		{61, InfectionBandCritical},
		{80, InfectionBandCritical},
		{81, InfectionBandRupture},
	}

	for _, tc := range cases {
		if got := InfectionBand(tc.infection); got != tc.expected {
			t.Fatalf("infection %.1f: expected %s, got %s", tc.infection, tc.expected, got)
		}
	}
}
