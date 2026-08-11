package player

import (
	"context"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/pkg/httputil"
)

const maxPlayerLevel = canon.PlayerMaxLevelMVP

type SkillDefinition struct {
	ID          string `json:"id"`
	Branch      string `json:"branch"`
	BranchName  string `json:"-"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Tier        int    `json:"tier"`
	MVP         bool   `json:"-"`
}

var skillDefinitions = []SkillDefinition{
	{ID: "defender_1", Branch: "defender", BranchName: "Защитник", Name: "Полевой каркас", Description: "Укрепляет human defensive posture в полевых операциях.", Tier: 1, MVP: true},
	{ID: "defender_2", Branch: "defender", BranchName: "Защитник", Name: "Купольная дисциплина", Description: "Усиливает удержание safe zone и оборонительный ритм.", Tier: 2, MVP: true},
	{ID: "defender_3", Branch: "defender", BranchName: "Защитник", Name: "Линия удержания", Description: "Поздний defensive perk для устойчивости frontline.", Tier: 3, MVP: true},
	{ID: "commander_1", Branch: "commander", BranchName: "Командир", Name: "Полевой приказ", Description: "Улучшает координацию squads и приоритеты задач.", Tier: 1, MVP: true},
	{ID: "commander_2", Branch: "commander", BranchName: "Командир", Name: "Маршевый ритм", Description: "Укрепляет темп human operations и возврат отрядов.", Tier: 2, MVP: true},
	{ID: "commander_3", Branch: "commander", BranchName: "Командир", Name: "Опорный контур", Description: "Поздний command perk для устойчивого боевого цикла.", Tier: 3, MVP: true},
	{ID: "energist_1", Branch: "energist", BranchName: "Энергетик", Name: "Резонансный узел", Description: "Post-MVP ветка управления энергосетью.", Tier: 1, MVP: false},
	{ID: "energist_2", Branch: "energist", BranchName: "Энергетик", Name: "Пиковый контур", Description: "Post-MVP ветка управления энергосетью.", Tier: 2, MVP: false},
}

func BuildProfilePayload(p *Player) map[string]any {
	xpMax := XPForNextLevel(p.Level)
	xp := p.XP
	if p.Level >= maxPlayerLevel {
		xpMax = XPForNextLevel(maxPlayerLevel)
		if xp > xpMax {
			xp = xpMax
		}
	}

	return map[string]any{
		"id":                       p.ID,
		"username":                 p.Username,
		"username_is_custom":       p.UsernameIsCustom,
		"onboarding_step":          p.OnboardingStep,
		"starter_beacon_available": p.StarterBeaconAvailable,
		"level":                    p.Level,
		"xp":                       xp,
		"xp_max":                   xpMax,
		"xp_next_level":            xpMax,
		"energy":                   p.Energy,
		"energy_max":               p.EnergyMax(),
		"materials":                p.Materials,
		"materials_max":            p.MaterialsMax(),
		"army_limit":               p.ArmyLimit,
		"crystals":                 p.Crystals,
		"symbiont_resonance":       p.SymbiontResonance,
		"skills":                   p.Skills,
		"pet_slots":                p.PetSlots,
		"skill_points_available":   availableSkillPoints(p),
		"position":                 p.Position,
		"beacons_placed":           0,
		"battles_won":              0,
		"km_walked":                0,
		"created_at":               p.CreatedAt,
	}
}

func (s *Service) GetSkillTree(ctx context.Context, playerID string) (map[string]any, error) {
	p, err := s.repo.GetByID(ctx, playerID)
	if err != nil {
		return nil, err
	}

	branches := []map[string]any{
		buildSkillBranch(p, "defender", "Защитник", p.Skills.Defender, true),
		buildSkillBranch(p, "commander", "Командир", p.Skills.Commander, true),
		buildSkillBranch(p, "energist", "Энергетик", p.Skills.Energist, false),
	}

	return map[string]any{
		"branches":               branches,
		"available_points":       availableSkillPoints(p),
		"player_level":           p.Level,
		"player_level_cap":       maxPlayerLevel,
		"active_release_profile": string(canon.CurrentReleaseStage),
	}, nil
}

func (s *Service) UnlockSkill(ctx context.Context, playerID, skillID string) (*Player, SkillDefinition, error) {
	p, err := s.repo.GetByID(ctx, playerID)
	if err != nil {
		return nil, SkillDefinition{}, err
	}

	def, ok := skillDefinitionByID(skillID)
	if !ok {
		return nil, SkillDefinition{}, httputil.NewNotFound("not_found", "skill not found")
	}
	if !def.MVP {
		return nil, SkillDefinition{}, httputil.NewBadRequest("feature_locked", "skill is not available in the current release")
	}
	if availableSkillPoints(p) <= 0 {
		return nil, SkillDefinition{}, httputil.NewBadRequest("no_skill_points", "no available skill points")
	}

	current := branchPoints(p.Skills, def.Branch)
	if def.Tier <= current {
		return nil, SkillDefinition{}, httputil.NewBadRequest("already_unlocked", "skill already unlocked")
	}
	if def.Tier != current+1 {
		return nil, SkillDefinition{}, httputil.NewBadRequest("prerequisite_missing", "unlock previous skill in branch first")
	}

	switch def.Branch {
	case "defender":
		p.Skills.Defender++
	case "commander":
		p.Skills.Commander++
	case "energist":
		p.Skills.Energist++
	default:
		return nil, SkillDefinition{}, httputil.NewBadRequest("invalid_skill", "unsupported skill branch")
	}

	if err := s.repo.Update(ctx, p); err != nil {
		return nil, SkillDefinition{}, err
	}
	return p, def, nil
}

func availableSkillPoints(p *Player) int {
	totalEarned := p.Level - 1
	if totalEarned < 0 {
		totalEarned = 0
	}
	totalSpent := p.Skills.Defender + p.Skills.Commander + p.Skills.Energist
	remaining := totalEarned - totalSpent
	if remaining < 0 {
		return 0
	}
	return remaining
}

func buildSkillBranch(p *Player, branchID, branchName string, unlockedCount int, available bool) map[string]any {
	skills := make([]map[string]any, 0, 3)
	for _, def := range skillDefinitions {
		if def.Branch != branchID {
			continue
		}
		skills = append(skills, map[string]any{
			"id":          def.ID,
			"name":        def.Name,
			"description": def.Description,
			"tier":        def.Tier,
			"unlocked":    def.Tier <= unlockedCount,
			"available":   available && def.Tier == unlockedCount+1 && availableSkillPoints(p) > 0,
		})
	}
	return map[string]any{
		"id":        branchID,
		"name":      branchName,
		"available": available,
		"spent":     unlockedCount,
		"skills":    skills,
	}
}

func skillDefinitionByID(skillID string) (SkillDefinition, bool) {
	for _, def := range skillDefinitions {
		if def.ID == skillID {
			return def, true
		}
	}
	return SkillDefinition{}, false
}

func branchPoints(skills SkillPoints, branch string) int {
	switch branch {
	case "defender":
		return skills.Defender
	case "commander":
		return skills.Commander
	case "energist":
		return skills.Energist
	default:
		return 0
	}
}
