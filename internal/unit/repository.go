package unit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the unit persistence interface.
type Repository interface {
	Create(ctx context.Context, u *Unit) error
	GetByID(ctx context.Context, id string) (*Unit, error)
	GetByPlayer(ctx context.Context, playerID string) ([]Unit, error)
	CountByPlayer(ctx context.Context, playerID string) (int, error)
	UpdateStatus(ctx context.Context, id, status string) error
	UpdateProgress(ctx context.Context, u *Unit) error
	AssignSquad(ctx context.Context, unitID string, squadID *string) error
	DeleteLostByPlayer(ctx context.Context, playerID string, count int) (int, error)
}

// PgRepository implements Repository with PostgreSQL.
type PgRepository struct {
	db *pgxpool.Pool
}

// NewPgRepository creates a new unit repository.
func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) Create(ctx context.Context, u *Unit) error {
	query := `
		INSERT INTO units (player_id, type, level, xp, hp, atk, squad_id, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`
	return r.db.QueryRow(ctx, query,
		u.PlayerID, u.Type, u.Level, u.XP, u.HP, u.ATK, u.SquadID, u.Status,
	).Scan(&u.ID)
}

func (r *PgRepository) GetByID(ctx context.Context, id string) (*Unit, error) {
	var u Unit
	query := `SELECT id, player_id, type, level, xp, hp, atk, squad_id, status FROM units WHERE id = $1`
	err := r.db.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.PlayerID, &u.Type, &u.Level, &u.XP,
		&u.HP, &u.ATK, &u.SquadID, &u.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("get unit: %w", err)
	}
	return &u, nil
}

func (r *PgRepository) GetByPlayer(ctx context.Context, playerID string) ([]Unit, error) {
	query := `SELECT id, player_id, type, level, xp, hp, atk, squad_id, status FROM units WHERE player_id = $1 AND status != 'lost' ORDER BY type, level DESC`
	rows, err := r.db.Query(ctx, query, playerID)
	if err != nil {
		return nil, fmt.Errorf("list units: %w", err)
	}
	defer rows.Close()

	var units []Unit
	for rows.Next() {
		var u Unit
		if err := rows.Scan(&u.ID, &u.PlayerID, &u.Type, &u.Level, &u.XP, &u.HP, &u.ATK, &u.SquadID, &u.Status); err != nil {
			return nil, fmt.Errorf("scan unit: %w", err)
		}
		units = append(units, u)
	}
	return units, nil
}

func (r *PgRepository) CountByPlayer(ctx context.Context, playerID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM units WHERE player_id = $1 AND status != 'lost'`, playerID).Scan(&count)
	return count, err
}

func (r *PgRepository) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := r.db.Exec(ctx, `UPDATE units SET status = $1 WHERE id = $2`, status, id)
	return err
}

// UpdateProgress persists a unit's level/xp and the HP/ATK rescaled on level-up.
func (r *PgRepository) UpdateProgress(ctx context.Context, u *Unit) error {
	_, err := r.db.Exec(ctx,
		`UPDATE units SET level = $1, xp = $2, hp = $3, atk = $4 WHERE id = $5`,
		u.Level, u.XP, u.HP, u.ATK, u.ID)
	return err
}

func (r *PgRepository) AssignSquad(ctx context.Context, unitID string, squadID *string) error {
	_, err := r.db.Exec(ctx, `UPDATE units SET squad_id = $1 WHERE id = $2`, squadID, unitID)
	return err
}

// DeleteLostByPlayer marks the oldest idle units as 'lost' and returns the number affected.
func (r *PgRepository) DeleteLostByPlayer(ctx context.Context, playerID string, count int) (int, error) {
	query := `
		UPDATE units SET status = 'lost'
		WHERE id IN (
			SELECT id FROM units
			WHERE player_id = $1 AND status = 'idle'
			ORDER BY level ASC, xp ASC
			LIMIT $2
		)`
	tag, err := r.db.Exec(ctx, query, playerID, count)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// HealAll restores every non-lost unit of a player to its level-appropriate max
// HP by type (shop item army_heal). The growth factor mirrors
// unit.LevelStatGrowth / StatsForLevel so a leveled unit is healed to its real
// max rather than its level-1 base.
func (r *PgRepository) HealAll(ctx context.Context, playerID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE units SET hp = round((CASE type
			WHEN 'fighter'  THEN $2
			WHEN 'engineer' THEN $3
			WHEN 'scout'    THEN $4
			WHEN 'medic'    THEN $5
			ELSE hp END) * (1 + $6 * (level - 1)))::int
		WHERE player_id = $1 AND status != 'lost'`,
		playerID,
		BaseStats["fighter"].HP, BaseStats["engineer"].HP,
		BaseStats["scout"].HP, BaseStats["medic"].HP,
		LevelStatGrowth,
	)
	return err
}
