package canon

import "testing"

func TestSpiritTameChance_RLGate(t *testing.T) {
	// Class V requires RL5 — below that, chance is 0 regardless of weakening.
	if c := SpiritTameChance(5, 4, 100); c != 0 {
		t.Fatalf("class V below RL gate must be 0, got %.2f", c)
	}
	if c := SpiritTameChance(5, 5, 0); c <= 0 {
		t.Fatalf("class V at RL5 should be tameable, got %.2f", c)
	}
}

func TestSpiritTameChance_WeakeningIsPity(t *testing.T) {
	// The more softened, the better the chance (weaken doubles as pity).
	low := SpiritTameChance(1, 1, 0)
	high := SpiritTameChance(1, 1, 100)
	if high <= low {
		t.Fatalf("weakening must raise tame chance: 0%%=%.2f 100%%=%.2f", low, high)
	}
	if high > 0.95 {
		t.Fatalf("tame chance must be capped at 0.95, got %.2f", high)
	}
}

func TestSpiritArchetype_Mapping(t *testing.T) {
	// Every class maps to a real archetype.
	for c := 1; c <= SpiritMaxClass; c++ {
		a := SpiritArchetype(c)
		if _, ok := EntitySpecFor(a); !ok {
			t.Fatalf("class %d → unknown archetype %q", c, a)
		}
	}
}

func TestSpiritConfig_VisibilityExceedsDanger(t *testing.T) {
	// §4.3: a spirit is visible before it is dangerous, for every class.
	for c := 1; c <= SpiritMaxClass; c++ {
		cfg := SpiritConfig(c)
		if cfg.VisibilityRadiusM <= cfg.DangerRadiusM {
			t.Fatalf("class %d: visibility %.0f must exceed danger %.0f", c, cfg.VisibilityRadiusM, cfg.DangerRadiusM)
		}
	}
}

func TestSpiritWaveDrain_ScalesAndClamps(t *testing.T) {
	if SpiritWaveDrainEnergy(5) <= SpiritWaveDrainEnergy(1) {
		t.Fatal("higher class must drain more")
	}
	if SpiritWaveDrainEnergy(99) != SpiritWaveDrainEnergy(5) {
		t.Fatal("class must clamp to max")
	}
}
