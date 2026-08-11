package pet

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines pet persistence operations.
type Repository interface {
	Create(ctx context.Context, p *Pet) error
	GetByID(ctx context.Context, id string) (*Pet, error)
	GetByPlayer(ctx context.Context, playerID string) ([]Pet, error)
	ListActiveMissions(ctx context.Context) ([]Pet, error)
	Update(ctx context.Context, p *Pet) error
}

// PgRepository implements Repository using PostgreSQL.
type PgRepository struct {
	db *pgxpool.Pool
}

// NewPgRepository creates a new PostgreSQL pet repository.
func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) Create(ctx context.Context, p *Pet) error {
	taskJSON, err := marshalTask(p.Task)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}
	query := `INSERT INTO pets (player_id, name, type, level, xp, status, task)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`
	return r.db.QueryRow(ctx, query, p.PlayerID, p.Name, p.Type, p.Level, p.XP, p.Status, taskJSON).
		Scan(&p.ID)
}

func (r *PgRepository) GetByID(ctx context.Context, id string) (*Pet, error) {
	query := `SELECT id, player_id, name, type, level, xp, status, task FROM pets WHERE id = $1`
	row := r.db.QueryRow(ctx, query, id)
	return scanPet(row)
}

func (r *PgRepository) GetByPlayer(ctx context.Context, playerID string) ([]Pet, error) {
	query := `SELECT id, player_id, name, type, level, xp, status, task FROM pets WHERE player_id = $1`
	rows, err := r.db.Query(ctx, query, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pets []Pet
	for rows.Next() {
		p, err := scanPetFromRows(rows)
		if err != nil {
			return nil, err
		}
		pets = append(pets, *p)
	}
	return pets, rows.Err()
}

// ListActiveMissions returns every pet currently on a task — the auto-claim
// worker filters by ReturnsAt in Go (the time lives inside the JSONB task).
func (r *PgRepository) ListActiveMissions(ctx context.Context) ([]Pet, error) {
	query := `SELECT id, player_id, name, type, level, xp, status, task FROM pets WHERE status = 'mission_active'`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pets []Pet
	for rows.Next() {
		p, err := scanPetFromRows(rows)
		if err != nil {
			return nil, err
		}
		pets = append(pets, *p)
	}
	return pets, rows.Err()
}

func (r *PgRepository) Update(ctx context.Context, p *Pet) error {
	taskJSON, err := marshalTask(p.Task)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}
	query := `UPDATE pets SET name=$1, type=$2, level=$3, xp=$4, status=$5, task=$6 WHERE id=$7`
	_, err = r.db.Exec(ctx, query, p.Name, p.Type, p.Level, p.XP, p.Status, taskJSON, p.ID)
	return err
}

// marshalTask converts PetTask to JSON bytes for JSONB storage.
func marshalTask(t *PetTask) ([]byte, error) {
	if t == nil {
		return nil, nil
	}
	return json.Marshal(t)
}

// scanPet scans a single pet row including JSONB task.
func scanPet(row pgx.Row) (*Pet, error) {
	var p Pet
	var taskJSON []byte
	err := row.Scan(&p.ID, &p.PlayerID, &p.Name, &p.Type, &p.Level, &p.XP, &p.Status, &taskJSON)
	if err != nil {
		return nil, fmt.Errorf("pet not found: %w", err)
	}
	if taskJSON != nil {
		var t PetTask
		if err := json.Unmarshal(taskJSON, &t); err != nil {
			return nil, fmt.Errorf("unmarshal task: %w", err)
		}
		p.Task = &t
	}
	return &p, nil
}

// scanPetFromRows scans a pet from an active Rows cursor.
func scanPetFromRows(rows pgx.Rows) (*Pet, error) {
	var p Pet
	var taskJSON []byte
	err := rows.Scan(&p.ID, &p.PlayerID, &p.Name, &p.Type, &p.Level, &p.XP, &p.Status, &taskJSON)
	if err != nil {
		return nil, fmt.Errorf("scan pet: %w", err)
	}
	if taskJSON != nil {
		var t PetTask
		if err := json.Unmarshal(taskJSON, &t); err != nil {
			return nil, fmt.Errorf("unmarshal task: %w", err)
		}
		p.Task = &t
	}
	return &p, nil
}

// CompleteActiveTask finishes the pet's current mission immediately by
// moving task.returns_at to now (shop item pet_speedup). No-op when the
// pet is idle.
func (r *PgRepository) CompleteActiveTask(ctx context.Context, playerID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE pets SET task = jsonb_set(task, '{returns_at}', to_jsonb(NOW()))
		WHERE player_id = $1 AND status = 'on_task' AND task IS NOT NULL`,
		playerID)
	return err
}
