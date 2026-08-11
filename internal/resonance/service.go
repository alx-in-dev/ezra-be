package resonance

import (
	"context"
	"fmt"

	"github.com/ezra-game/server/internal/item"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/pkg/httputil"
)

// ItemStore reads and consumes the player's resonance signatures.
type ItemStore interface {
	ListByPlayer(ctx context.Context, playerID string) ([]item.Item, error)
	Grant(ctx context.Context, playerID, itemType, variant string, delta int) (int, error)
}

// PlayerStore provides the player's current position for the wave epicentre.
type PlayerStore interface {
	GetByID(ctx context.Context, id string) (*player.Player, error)
}

// Service implements the D1 resonance endgame: collect the full signature
// spectrum, then launch a permanent regional counter-wave.
type Service struct {
	items   ItemStore
	players PlayerStore
	repo    Repository
}

func NewService(items ItemStore, players PlayerStore, repo Repository) *Service {
	return &Service{items: items, players: players, repo: repo}
}

// Status reports the player's spectrum collection and wave history.
func (s *Service) Status(ctx context.Context, playerID string) (*Status, error) {
	items, err := s.items.ListByPlayer(ctx, playerID)
	if err != nil {
		return nil, err
	}
	owned := map[string]int{}
	for _, it := range items {
		if it.ItemType == item.TypeResonanceSignature {
			owned[it.Variant] += it.Qty
		}
	}
	spectra := make([]SpectrumStatus, 0, len(item.ResonanceSpectra))
	collected := 0
	for _, key := range item.ResonanceSpectra {
		q := owned[key]
		have := q > 0
		if have {
			collected++
		}
		spectra = append(spectra, SpectrumStatus{Key: key, Name: item.SpectrumRu(key), Owned: q, Have: have})
	}
	waves, err := s.repo.WaveCount(ctx, playerID)
	if err != nil {
		return nil, err
	}
	return &Status{
		Spectra:       spectra,
		Complete:      collected == len(item.ResonanceSpectra) && len(item.ResonanceSpectra) > 0,
		Collected:     collected,
		Total:         len(item.ResonanceSpectra),
		WavesLaunched: waves,
	}, nil
}

// Activate consumes one signature of every spectrum and launches a counter-wave
// centred on the player, permanently stabilizing the surrounding cells.
func (s *Service) Activate(ctx context.Context, playerID string) (*ActivateResult, error) {
	st, err := s.Status(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if !st.Complete {
		return nil, httputil.NewBadRequest("incomplete_set",
			fmt.Sprintf("собран %d/%d спектров резонанса — нужно собрать весь набор", st.Collected, st.Total))
	}
	p, err := s.players.GetByID(ctx, playerID)
	if err != nil {
		return nil, httputil.NewInternal("failed to get player")
	}

	// Consume one signature of each spectrum (the cost of the wave).
	for _, key := range item.ResonanceSpectra {
		if _, err := s.items.Grant(ctx, playerID, item.TypeResonanceSignature, key, -1); err != nil {
			return nil, fmt.Errorf("consume signature %s: %w", key, err)
		}
	}

	cells, err := s.repo.StabilizeRadius(ctx, playerID, p.Position.Lat, p.Position.Lng, CounterWaveRadiusM)
	if err != nil {
		return nil, err
	}
	if err := s.repo.RecordWave(ctx, playerID, p.Position.Lat, p.Position.Lng, CounterWaveRadiusM, cells); err != nil {
		return nil, err
	}
	waves, _ := s.repo.WaveCount(ctx, playerID)
	return &ActivateResult{
		CellsStabilized: cells,
		WaveNumber:      waves,
		Lat:             p.Position.Lat,
		Lng:             p.Position.Lng,
		RadiusM:         CounterWaveRadiusM,
	}, nil
}
