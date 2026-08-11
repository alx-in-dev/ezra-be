package tower

import (
	"context"
	"testing"

	"github.com/ezra-game/server/internal/player"
	"github.com/stretchr/testify/assert"
)

type fakeDomeArea struct{ counts map[string]int }

func (f *fakeDomeArea) SealedCellCountsByPlayer(_ context.Context) (map[string]int, error) {
	return f.counts, nil
}

func TestHandleAccruePassiveIncome_AddsDomeAreaIncome(t *testing.T) {
	ctx := context.Background()
	towerRepo := &mockTowerRepo{}
	playerRepo := &mockPlayerRepo{}
	resourceSvc := player.NewResourceService(playerRepo)
	// 20 sealed cells → round(0.3 × 20) = 6 area energy on top of the L1 beacon.
	worker := NewWorker(towerRepo, resourceSvc).
		WithDomeArea(&fakeDomeArea{counts: map[string]int{"player-1": 20}})

	towerRepo.On("GetAll", ctx).Return([]Tower{
		{ID: "t1", OwnerID: "player-1", Level: 1},
	}, nil)
	p := &player.Player{ID: "player-1", Energy: 100, Materials: 0}
	playerRepo.On("GetByID", ctx, "player-1").Return(p, nil)
	playerRepo.On("Update", ctx, p).Return(nil)

	err := worker.HandleAccruePassiveIncome(ctx, NewAccruePassiveIncomeTask())

	assert.NoError(t, err)
	// L1 beacon energy 2 + area income 6 = 8 → 100 + 8 = 108.
	assert.Equal(t, 108, p.Energy)
}

func TestHandleAccruePassiveIncome_AggregatesByPlayer(t *testing.T) {
	ctx := context.Background()
	towerRepo := &mockTowerRepo{}
	playerRepo := &mockPlayerRepo{}
	resourceSvc := player.NewResourceService(playerRepo)
	worker := NewWorker(towerRepo, resourceSvc)

	towerRepo.On("GetAll", ctx).Return([]Tower{
		{ID: "t1", OwnerID: "player-1", Level: 1},
		{ID: "t2", OwnerID: "player-1", Level: 3},
		{ID: "t3", OwnerID: "player-2", Level: 2},
	}, nil)

	playerOne := &player.Player{ID: "player-1", Energy: 100, Materials: 20}
	playerTwo := &player.Player{ID: "player-2", Energy: 50, Materials: 5}
	playerRepo.On("GetByID", ctx, "player-1").Return(playerOne, nil)
	playerRepo.On("GetByID", ctx, "player-2").Return(playerTwo, nil)
	playerRepo.On("Update", ctx, playerOne).Return(nil)
	playerRepo.On("Update", ctx, playerTwo).Return(nil)

	err := worker.HandleAccruePassiveIncome(ctx, NewAccruePassiveIncomeTask())

	assert.NoError(t, err)
	assert.Equal(t, 109, playerOne.Energy)
	// C5: L1 + L3 now each yield 1 material/hour → +2.
	assert.Equal(t, 22, playerOne.Materials)
	assert.Equal(t, 54, playerTwo.Energy)
	// C5: L2 now yields 1 material/hour → +1.
	assert.Equal(t, 6, playerTwo.Materials)
}
