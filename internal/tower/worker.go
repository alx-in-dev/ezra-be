package tower

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"

	"github.com/ezra-game/server/internal/canon"
	"github.com/ezra-game/server/internal/player"
	"github.com/hibiken/asynq"
)

const TypeAccruePassiveIncome = "tower:accrue_passive_income"

// TypePressureTick grinds beacon HP by cell infection (PvE pressure on the
// network); the asynq handler delegates to tower.Service.ApplyPressure.
const TypePressureTick = "tower:pressure_tick"

// NewPressureTask builds the recurring beacon-pressure task.
func NewPressureTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypePressureTick, payload)
}

// DomeAreaReader supplies how many sealed-dome cells each player has, so the
// passive-income worker can credit area income (B3). Optional / nil-safe.
type DomeAreaReader interface {
	SealedCellCountsByPlayer(ctx context.Context) (map[string]int, error)
}

type Worker struct {
	towers    Repository
	resources *player.ResourceService
	domeArea  DomeAreaReader
}

func NewWorker(towers Repository, resources *player.ResourceService) *Worker {
	return &Worker{towers: towers, resources: resources}
}

// WithDomeArea wires the sealed-dome area income source (B3). Returns the worker
// for chaining; nil-safe (no reader → no area income).
func (w *Worker) WithDomeArea(d DomeAreaReader) *Worker {
	w.domeArea = d
	return w
}

func (w *Worker) HandleAccruePassiveIncome(ctx context.Context, _ *asynq.Task) error {
	// Legacy beacons (owner went Symbiont) pay the ex-owner nothing — prefer
	// the filtered query when the repo provides it (tests' fakes may not).
	var allTowers []Tower
	var err error
	if lister, ok := w.towers.(interface {
		GetIncomeEligible(ctx context.Context) ([]Tower, error)
	}); ok {
		allTowers, err = lister.GetIncomeEligible(ctx)
	} else {
		allTowers, err = w.towers.GetAll(ctx)
	}
	if err != nil {
		return err
	}

	type income struct {
		energy    int
		materials int
	}
	perPlayer := map[string]income{}

	for _, tower := range allTowers {
		cfg, ok := LevelConfigs[tower.Level]
		if !ok || tower.OwnerID == "" {
			continue
		}

		energy, materials := cfg.EnergyPerHour, cfg.MaterialsPerHour
		// Brownout halves income (beacon_network_dome.md §5) — over-budget
		// networks pay for the stretch.
		if tower.Brownout {
			energy /= 2
			materials /= 2
		}
		current := perPlayer[tower.OwnerID]
		current.energy += energy
		current.materials += materials
		perPlayer[tower.OwnerID] = current
	}

	// Dome area income (B3): sealed-contour cells pay energy per cell/hour, on
	// top of per-beacon income (sealed cells only → no double counting with the
	// radius/disk field).
	if w.domeArea != nil {
		sealed, err := w.domeArea.SealedCellCountsByPlayer(ctx)
		if err != nil {
			slog.Warn("passive income: sealed cell counts failed", "error", err)
		} else {
			for playerID, cells := range sealed {
				areaEnergy := int(math.Round(canon.DomeAreaEnergyPerCellPerHour * float64(cells)))
				if areaEnergy == 0 {
					continue
				}
				current := perPlayer[playerID]
				current.energy += areaEnergy
				perPlayer[playerID] = current
			}
		}
	}

	credited := 0
	for playerID, hourlyIncome := range perPlayer {
		// AddPassiveIncome enforces storage cap (via min()) AND the 24h
		// offline window — paused players get a (nil, nil) no-op.
		p, err := w.resources.AddPassiveIncome(ctx, playerID, hourlyIncome.energy, hourlyIncome.materials)
		if err != nil {
			return err
		}
		if p != nil {
			credited++
		}
	}

	slog.Info("tower passive income accrued",
		"players_total", len(perPlayer),
		"players_credited", credited,
		"players_paused", len(perPlayer)-credited)
	return nil
}

func NewAccruePassiveIncomeTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeAccruePassiveIncome, payload)
}
