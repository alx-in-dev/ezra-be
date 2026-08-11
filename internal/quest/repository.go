package quest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// QuestRepository defines daily quest persistence operations.
type QuestRepository interface {
	Create(ctx context.Context, q *DailyQuest) error
	GetByPlayerAndDate(ctx context.Context, playerID string, date time.Time) ([]DailyQuest, error)
	UpdateProgress(ctx context.Context, id string, progress int) error
	Claim(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*DailyQuest, error)
}

// StreakRepository defines streak persistence operations.
type StreakRepository interface {
	Get(ctx context.Context, playerID string) (*Streak, error)
	Upsert(ctx context.Context, s *Streak) error
}

// PgQuestRepository implements QuestRepository using PostgreSQL.
type PgQuestRepository struct {
	db *pgxpool.Pool
}

// NewPgQuestRepository creates a new PostgreSQL quest repository.
func NewPgQuestRepository(db *pgxpool.Pool) *PgQuestRepository {
	return &PgQuestRepository{db: db}
}

func (r *PgQuestRepository) Create(ctx context.Context, q *DailyQuest) error {
	rewardJSON, err := json.Marshal(q.Reward)
	if err != nil {
		return fmt.Errorf("marshal reward: %w", err)
	}
	query := `INSERT INTO daily_quests (player_id, type, description, progress, target, reward, date, claimed)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`
	return r.db.QueryRow(ctx, query,
		q.PlayerID, q.Type, q.Description, q.Progress, q.Target, rewardJSON, q.Date, q.Claimed,
	).Scan(&q.ID)
}

func (r *PgQuestRepository) GetByPlayerAndDate(ctx context.Context, playerID string, date time.Time) ([]DailyQuest, error) {
	query := `SELECT id, player_id, type, description, progress, target, reward, date, claimed
		FROM daily_quests WHERE player_id = $1 AND date = $2`
	rows, err := r.db.Query(ctx, query, playerID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var quests []DailyQuest
	for rows.Next() {
		q, err := scanQuest(rows)
		if err != nil {
			return nil, err
		}
		quests = append(quests, *q)
	}
	return quests, rows.Err()
}

func (r *PgQuestRepository) UpdateProgress(ctx context.Context, id string, progress int) error {
	_, err := r.db.Exec(ctx, `UPDATE daily_quests SET progress = $1 WHERE id = $2`, progress, id)
	return err
}

func (r *PgQuestRepository) Claim(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE daily_quests SET claimed = TRUE WHERE id = $1`, id)
	return err
}

func (r *PgQuestRepository) GetByID(ctx context.Context, id string) (*DailyQuest, error) {
	query := `SELECT id, player_id, type, description, progress, target, reward, date, claimed
		FROM daily_quests WHERE id = $1`
	row := r.db.QueryRow(ctx, query, id)

	var q DailyQuest
	var rewardJSON []byte
	err := row.Scan(&q.ID, &q.PlayerID, &q.Type, &q.Description, &q.Progress, &q.Target, &rewardJSON, &q.Date, &q.Claimed)
	if err != nil {
		return nil, fmt.Errorf("quest not found: %w", err)
	}
	if err := json.Unmarshal(rewardJSON, &q.Reward); err != nil {
		return nil, fmt.Errorf("unmarshal reward: %w", err)
	}
	return &q, nil
}

// scanQuest scans a DailyQuest from an active Rows cursor.
func scanQuest(rows pgx.Rows) (*DailyQuest, error) {
	var q DailyQuest
	var rewardJSON []byte
	err := rows.Scan(&q.ID, &q.PlayerID, &q.Type, &q.Description, &q.Progress, &q.Target, &rewardJSON, &q.Date, &q.Claimed)
	if err != nil {
		return nil, fmt.Errorf("scan quest: %w", err)
	}
	if err := json.Unmarshal(rewardJSON, &q.Reward); err != nil {
		return nil, fmt.Errorf("unmarshal reward: %w", err)
	}
	return &q, nil
}

// PgStreakRepository implements StreakRepository using PostgreSQL.
type PgStreakRepository struct {
	db *pgxpool.Pool
}

// NewPgStreakRepository creates a new PostgreSQL streak repository.
func NewPgStreakRepository(db *pgxpool.Pool) *PgStreakRepository {
	return &PgStreakRepository{db: db}
}

func (r *PgStreakRepository) Get(ctx context.Context, playerID string) (*Streak, error) {
	query := `SELECT player_id, days, last_checkin FROM streaks WHERE player_id = $1`
	var s Streak
	err := r.db.QueryRow(ctx, query, playerID).Scan(&s.PlayerID, &s.Days, &s.LastCheckin)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &Streak{PlayerID: playerID, Days: 0, LastCheckin: nil}, nil
		}
		return nil, err
	}
	return &s, nil
}

func (r *PgStreakRepository) Upsert(ctx context.Context, s *Streak) error {
	query := `INSERT INTO streaks (player_id, days, last_checkin)
		VALUES ($1, $2, $3)
		ON CONFLICT (player_id) DO UPDATE SET days = $2, last_checkin = $3`
	_, err := r.db.Exec(ctx, query, s.PlayerID, s.Days, s.LastCheckin)
	return err
}
