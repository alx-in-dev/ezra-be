package nest

import (
	"context"

	"github.com/ezra-game/server/internal/canon"
)

// DefenderReader reads a player's Defender skill points (T-844). Implemented by
// a thin adapter over the player repository in main — kept an interface so the
// nest package doesn't import player.
type DefenderReader interface {
	DefenderPoints(ctx context.Context, playerID string) (int, error)
}

// skillDefenderModifier lengthens the siege window by the owner's Defender skill
// (T-844): the human skill tree's Defender branch, previously effect-less, now
// hardens a Symbiont nest's defense. Registered as a DefenseModifier (ADR-N3-5),
// so it composes with the N4 spirit-garrison modifier without any state-machine
// change.
type skillDefenderModifier struct {
	reader   DefenderReader
	perPoint float64
}

// NewSkillDefenderModifier builds the Defender-skill defense modifier (T-844).
func NewSkillDefenderModifier(r DefenderReader) DefenseModifier {
	return skillDefenderModifier{reader: r, perPoint: canon.NestDefenderWindowPerPoint}
}

func (m skillDefenderModifier) DefenseMultiplier(ctx context.Context, n *Nest) (float64, error) {
	pts, err := m.reader.DefenderPoints(ctx, n.OwnerID)
	if err != nil {
		return 1, err
	}
	if pts < 0 {
		pts = 0
	}
	return 1 + m.perPoint*float64(pts), nil
}

// GarrisonReader reports how many tamed spirits the owner is keeping HOME (idle /
// available — not dispatched to raid) to defend the nest (T-873). Implemented by
// a thin adapter over the roster entity repository in main — kept an interface so
// the nest package doesn't import roster.
type GarrisonReader interface {
	HomeGarrison(ctx context.Context, playerID string) (int, error)
}

// spiritGarrisonModifier lengthens the siege window by the owner's home garrison
// of tamed spirits (T-873): the N4 payoff of the weaken→tame loop, and the
// strategic tension of holding the swarm home vs. sending it to raid human
// beacons. Composes with skillDefenderModifier through the same DefenseModifier
// seam (ADR-N3-5) — no state-machine change. Capped (canon.NestGarrisonCap) so a
// big roster can't make a nest unbreakable (venue-safety / П2).
type spiritGarrisonModifier struct {
	reader  GarrisonReader
	perUnit float64
	cap     int
}

// NewSpiritGarrisonModifier builds the N4 spirit-garrison defense modifier (T-873).
func NewSpiritGarrisonModifier(r GarrisonReader) DefenseModifier {
	return spiritGarrisonModifier{reader: r, perUnit: canon.NestGarrisonWindowPerSpirit, cap: canon.NestGarrisonCap}
}

func (m spiritGarrisonModifier) DefenseMultiplier(ctx context.Context, n *Nest) (float64, error) {
	g, err := m.reader.HomeGarrison(ctx, n.OwnerID)
	if err != nil {
		return 1, err
	}
	if g < 0 {
		g = 0
	}
	if g > m.cap {
		g = m.cap
	}
	return 1 + m.perUnit*float64(g), nil
}
