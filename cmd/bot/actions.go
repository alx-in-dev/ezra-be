package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ezra-game/server/internal/network"
	"github.com/ezra-game/server/internal/pet"
	"github.com/ezra-game/server/internal/quest"
	"github.com/ezra-game/server/internal/shop"
	"github.com/ezra-game/server/internal/squad"
	"github.com/ezra-game/server/internal/survivor"
)

// idItem is the minimal shape we need from list endpoints: just the id (plus a
// couple of optional fields some callers read).
type idItem struct {
	ID    string `json:"id"`
	Level int    `json:"level"`
}

// --- Network / core (placing a core domes the surrounding cells) -----------

func (c *Client) PlaceCore(ctx context.Context, cellID string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/core", network.CoreRequest{CellID: cellID}, nil)
}

func (c *Client) UpgradeCore(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/v1/core/upgrade", nil, nil)
}

// --- Tower lifecycle --------------------------------------------------------

func (c *Client) TowersMine(ctx context.Context) ([]idItem, error) {
	var out struct {
		Towers []idItem `json:"towers"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/towers/mine", nil, &out)
	return out.Towers, err
}

func (c *Client) TowerUpgrade(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPatch, "/api/v1/towers/"+id+"/upgrade", nil, nil)
}

func (c *Client) TowerRepair(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/towers/"+id+"/repair", nil, nil)
}

func (c *Client) ForceCapture(ctx context.Context, towerID string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/towers/"+towerID+"/capture/force", nil, nil)
}

// --- Skills -----------------------------------------------------------------

func (c *Client) UnlockSkill(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/skills/"+id+"/unlock", nil, nil)
}

// --- Quests / streak --------------------------------------------------------

func (c *Client) GetQuests(ctx context.Context) ([]quest.DailyQuest, error) {
	var out struct {
		Quests []quest.DailyQuest `json:"quests"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/quests", nil, &out)
	return out.Quests, err
}

func (c *Client) ClaimQuest(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/quests/"+id+"/claim", nil, nil)
}

func (c *Client) StreakCheckin(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/v1/streaks/checkin", nil, nil)
}

// --- Pets -------------------------------------------------------------------

func (c *Client) GetPets(ctx context.Context) ([]idItem, error) {
	var out struct {
		Pets []idItem `json:"pets"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/pets", nil, &out)
	return out.Pets, err
}

func (c *Client) ClaimStarterPet(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/v1/pets/starter-claim", nil, nil)
}

func (c *Client) SendPet(ctx context.Context, id, task string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/pets/"+id+"/send", pet.SendRequest{TaskType: task}, nil)
}

func (c *Client) RecallPet(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/pets/"+id+"/recall", nil, nil)
}

// --- Squad missions ---------------------------------------------------------

func (c *Client) SendSquad(ctx context.Context, id, mission, target string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/squads/"+id+"/send",
		squad.SendRequest{MissionType: mission, TargetID: target}, nil)
}

func (c *Client) RecallSquad(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/squads/"+id+"/recall", nil, nil)
}

// --- Survivors --------------------------------------------------------------

func (c *Client) RecruitSurvivor(ctx context.Context, sType string, lat, lng float64) error {
	return c.do(ctx, http.MethodPost, "/api/v1/survivors/recruit",
		survivor.RecruitRequest{Type: sType, Lat: lat, Lng: lng}, nil)
}

// --- Hives (symbiont empower) ----------------------------------------------

func (c *Client) GetHives(ctx context.Context) ([]idItem, error) {
	var out struct {
		Hives []idItem `json:"hives"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/hives", nil, &out)
	return out.Hives, err
}

func (c *Client) EmpowerHive(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/hives/"+id+"/empower", nil, nil)
}

// --- Spire / resonance ------------------------------------------------------

func (c *Client) SpireContribute(ctx context.Context, lat, lng float64) error {
	return c.do(ctx, http.MethodPost,
		fmt.Sprintf("/api/v1/spire/contribute?lat=%f&lng=%f", lat, lng), nil, nil)
}

func (c *Client) ResonanceActivate(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/v1/resonance/activate", nil, nil)
}

// --- Battle overcharge ------------------------------------------------------

func (c *Client) Overcharge(ctx context.Context, battleID string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/battles/"+battleID+"/overcharge", nil, nil)
}

// --- Shop -------------------------------------------------------------------

func (c *Client) ShopCatalog(ctx context.Context) ([]idItem, error) {
	var out struct {
		Items []idItem `json:"items"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/shop/catalog", nil, &out)
	return out.Items, err
}

func (c *Client) ShopBuy(ctx context.Context, itemID string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/shop/buy", shop.BuyRequest{ItemID: itemID}, nil)
}

func (c *Client) ShopAddCrystals(ctx context.Context, amount int) error {
	return c.do(ctx, http.MethodPost, "/api/v1/shop/crystals",
		shop.AddCrystalsRequest{Amount: amount, Receipt: "dev", Platform: "android"}, nil)
}
