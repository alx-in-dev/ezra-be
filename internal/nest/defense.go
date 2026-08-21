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

// Commander → tamed-spirit control cap (T-846) is intentionally NOT wired here:
// spirits do not exist until N4. The branch is documented as N4-gated so it
// shows no dead effect now (anti-C1 "dead signals") and lights up when the
// spirit garrison (N4) registers its own modifier / cap reader.
