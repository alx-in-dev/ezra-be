package player

import "github.com/ezra-game/server/internal/canon"

func BuildOnboardingPayload(p *Player) map[string]any {
	step := p.OnboardingStep
	if step == "" {
		step = canon.OnboardingNotStarted
	}

	return map[string]any{
		"step":                     step,
		"username_required":        !p.UsernameIsCustom,
		"starter_beacon_available": p.StarterBeaconAvailable,
		"is_completed":             step == canon.OnboardingCompleted,
	}
}
