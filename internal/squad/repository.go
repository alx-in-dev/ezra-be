package squad

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the squad persistence interface.
type Repository interface {
	Create(ctx context.Context, s *Squad) error
	GetByID(ctx context.Context, id string) (*Squad, error)
	GetByPlayer(ctx context.Context, playerID string) ([]Squad, error)
	Update(ctx context.Context, s *Squad) error
	Delete(ctx context.Context, id string) error
	GetDueMissions(ctx context.Context, before time.Time) ([]Squad, error)
}

// PgRepository implements Repository with PostgreSQL.
type PgRepository struct {
	db *pgxpool.Pool
}

// NewPgRepository creates a new squad repository.
func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) Create(ctx context.Context, s *Squad) error {
	missionJSON, _ := marshalMission(s.Mission)

	query := `
		INSERT INTO squads (player_id, name, status, mission)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`
	return r.db.QueryRow(ctx, query,
		s.PlayerID, s.Name, s.Status, missionJSON,
	).Scan(&s.ID, &s.CreatedAt)
}

func (r *PgRepository) GetByID(ctx context.Context, id string) (*Squad, error) {
	var s Squad
	var missionJSON []byte
	var createdAt time.Time

	query := `SELECT id, player_id, name, status, mission, created_at FROM squads WHERE id = $1`
	err := r.db.QueryRow(ctx, query, id).Scan(
		&s.ID, &s.PlayerID, &s.Name, &s.Status, &missionJSON, &createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get squad: %w", err)
	}

	s.CreatedAt = createdAt
	s.Mission = unmarshalMission(missionJSON)
	return &s, nil
}

func (r *PgRepository) GetByPlayer(ctx context.Context, playerID string) ([]Squad, error) {
	query := `SELECT id, player_id, name, status, mission, created_at FROM squads WHERE player_id = $1 ORDER BY created_at DESC`
	rows, err := r.db.Query(ctx, query, playerID)
	if err != nil {
		return nil, fmt.Errorf("list squads: %w", err)
	}
	defer rows.Close()

	var squads []Squad
	for rows.Next() {
		var s Squad
		var missionJSON []byte
		if err := rows.Scan(&s.ID, &s.PlayerID, &s.Name, &s.Status, &missionJSON, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan squad: %w", err)
		}
		s.Mission = unmarshalMission(missionJSON)
		squads = append(squads, s)
	}
	return squads, nil
}

func (r *PgRepository) Update(ctx context.Context, s *Squad) error {
	missionJSON, _ := marshalMission(s.Mission)

	query := `UPDATE squads SET name = $1, status = $2, mission = $3 WHERE id = $4`
	_, err := r.db.Exec(ctx, query, s.Name, s.Status, missionJSON, s.ID)
	return err
}

func (r *PgRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM squads WHERE id = $1`, id)
	return err
}

// GetDueMissions returns squads on a timed mission whose ReturnsAt has passed.
func (r *PgRepository) GetDueMissions(ctx context.Context, before time.Time) ([]Squad, error) {
	query := `SELECT id, player_id, name, status, mission, created_at FROM squads
		WHERE status = 'on_mission'
		  AND mission IS NOT NULL
		  AND (mission->>'returns_at')::timestamptz <= $1`
	rows, err := r.db.Query(ctx, query, before)
	if err != nil {
		return nil, fmt.Errorf("get due missions: %w", err)
	}
	defer rows.Close()

	var squads []Squad
	for rows.Next() {
		var s Squad
		var missionJSON []byte
		if err := rows.Scan(&s.ID, &s.PlayerID, &s.Name, &s.Status, &missionJSON, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan due squad: %w", err)
		}
		s.Mission = unmarshalMission(missionJSON)
		squads = append(squads, s)
	}
	return squads, rows.Err()
}

// marshalMission converts a Mission pointer to JSONB-compatible []byte.
func marshalMission(m *Mission) ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// unmarshalMission converts JSONB bytes to a Mission pointer.
func unmarshalMission(data []byte) *Mission {
	if len(data) == 0 {
		return nil
	}
	var m Mission
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return &m
}
