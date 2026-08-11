package unit

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

const TypeArmyDecay = "army:decay"

// Worker handles periodic army decay tasks.
type Worker struct {
	db    *pgxpool.Pool
	units Repository
}

// NewWorker creates a new army decay worker.
func NewWorker(db *pgxpool.Pool, units Repository) *Worker {
	return &Worker{db: db, units: units}
}

type inactivePlayer struct {
	ID         string
	LastActive time.Time
	UnitCount  int
}

// HandleDecay processes army decay for inactive players.
// For each player with last_active > 24h: lose floor(count * 0.05 * hours/24), max 30%.
// 7d+ inactive → freeze (skip).
func (w *Worker) HandleDecay(ctx context.Context, _ *asynq.Task) error {
	slog.Info("army decay started")

	// Find players with units who have been inactive > 24h but < 7d
	query := `
		SELECT p.id, p.last_active, COUNT(u.id) AS unit_count
		FROM players p
		JOIN units u ON u.player_id = p.id AND u.status = 'idle'
		WHERE p.last_active < NOW() - INTERVAL '24 hours'
		  AND p.last_active >= NOW() - INTERVAL '7 days'
		GROUP BY p.id, p.last_active
		HAVING COUNT(u.id) > 0`

	rows, err := w.db.Query(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var players []inactivePlayer
	for rows.Next() {
		var ip inactivePlayer
		if err := rows.Scan(&ip.ID, &ip.LastActive, &ip.UnitCount); err != nil {
			slog.Error("scan inactive player", "error", err)
			continue
		}
		players = append(players, ip)
	}

	totalLost := 0
	for _, ip := range players {
		hoursInactive := time.Since(ip.LastActive).Hours()
		lossCount := int(math.Floor(float64(ip.UnitCount) * 0.05 * hoursInactive / 24.0))

		// Cap at 30% of army
		maxLoss := int(math.Floor(float64(ip.UnitCount) * 0.30))
		if lossCount > maxLoss {
			lossCount = maxLoss
		}

		if lossCount <= 0 {
			continue
		}

		lost, err := w.units.DeleteLostByPlayer(ctx, ip.ID, lossCount)
		if err != nil {
			slog.Error("army decay failed", "player_id", ip.ID, "error", err)
			continue
		}
		totalLost += lost
		slog.Info("army decay applied", "player_id", ip.ID, "units_lost", lost)
	}

	slog.Info("army decay completed", "players_processed", len(players), "total_units_lost", totalLost)
	return nil
}

// NewDecayTask creates a new army:decay asynq task.
func NewDecayTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeArmyDecay, payload)
}
