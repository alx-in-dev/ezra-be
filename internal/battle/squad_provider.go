package battle

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/pkg/httputil"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgSquadProvider implements SquadProvider using PostgreSQL.
type PgSquadProvider struct {
	db *pgxpool.Pool
}

// NewPgSquadProvider creates a new squad provider.
func NewPgSquadProvider(db *pgxpool.Pool) *PgSquadProvider {
	return &PgSquadProvider{db: db}
}

func (p *PgSquadProvider) GetSquadOwner(ctx context.Context, squadID string) (string, error) {
	var ownerID string
	err := p.db.QueryRow(ctx, `SELECT player_id FROM squads WHERE id = $1`, squadID).Scan(&ownerID)
	if err != nil {
		return "", fmt.Errorf("squad not found: %w", err)
	}
	return ownerID, nil
}

func (p *PgSquadProvider) GetSquadUnits(ctx context.Context, squadID string) ([]SquadUnit, error) {
	rows, err := p.db.Query(ctx,
		`SELECT id, type, atk, hp FROM units WHERE squad_id = $1 AND status != 'lost' ORDER BY id`, squadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var units []SquadUnit
	for rows.Next() {
		var u SquadUnit
		if err := rows.Scan(&u.ID, &u.Type, &u.ATK, &u.HP); err != nil {
			return nil, err
		}
		units = append(units, u)
	}
	return units, rows.Err()
}

func (p *PgSquadProvider) StartMission(ctx context.Context, squadID, targetType, targetID string) error {
	var status string
	err := p.db.QueryRow(ctx, `SELECT status FROM squads WHERE id = $1`, squadID).Scan(&status)
	if err != nil {
		return fmt.Errorf("get squad status: %w", err)
	}
	if status != "idle" {
		return httputil.NewBadRequest("squad_busy", "squad is already on a mission")
	}

	// Battle-driven missions must NOT reuse the timed-mission types
	// (patrol/capture): the mission worker pays those on return, and battle
	// loot is paid by the battle itself — an abandoned fight would double-dip.
	missionType := "attack_rift"
	switch targetType {
	case "rift":
		missionType = "attack_rift"
	case "hive":
		missionType = "assault_hive"
	default:
		missionType = "battle"
	}
	missionJSON, err := json.Marshal(map[string]any{
		"type":       missionType,
		"target_id":  targetID,
		"started_at": time.Now(),
		"returns_at": time.Now().Add(canon.SquadMissionDuration),
	})
	if err != nil {
		return fmt.Errorf("marshal mission: %w", err)
	}

	_, err = p.db.Exec(ctx, `UPDATE squads SET status = 'on_mission', mission = $1 WHERE id = $2`, missionJSON, squadID)
	return err
}

func (p *PgSquadProvider) FinishMission(ctx context.Context, squadID string) error {
	_, err := p.db.Exec(ctx, `UPDATE squads SET status = 'idle', mission = NULL WHERE id = $1`, squadID)
	return err
}

func (p *PgSquadProvider) MarkUnitsLost(ctx context.Context, unitIDs []string) error {
	if len(unitIDs) == 0 {
		return nil
	}
	_, err := p.db.Exec(ctx,
		`UPDATE units SET status = 'lost', squad_id = NULL WHERE id = ANY($1)`, unitIDs)
	return err
}
