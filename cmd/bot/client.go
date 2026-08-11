package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ezra-game/server/internal/battle"
	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/faction"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/pvp"
	"github.com/ezra-game/server/internal/squad"
	"github.com/ezra-game/server/internal/tower"
	"github.com/ezra-game/server/internal/unit"
)

// Client is a thin REST client over the Ezra game API, scoped to one bot's
// session token. It deliberately reuses the server's own request/response types
// (player.Player, cell.CellDTO, tower.PlaceRequest, ...) so the compiler flags
// any drift between the bot and the live API contract.
type Client struct {
	base  string
	token string
	http  *http.Client
}

func NewClient(base string) *Client {
	// Timeout is generous because GetCells can trigger a lazy server-side
	// Overpass region seed on first visit to a fresh 0.1° grid cell (~20-30s).
	return &Client{base: base, http: &http.Client{Timeout: 45 * time.Second}}
}

// apiError carries the HTTP status and raw body for failed calls.
type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("http %d: %s", e.Status, e.Body) }

// authResponse mirrors auth.AuthResponse (Player is opaque here — we re-fetch it
// via GetPlayer to decode into the typed model).
type authResponse struct {
	SessionToken string `json:"session_token"`
}

// EnsureAuth registers the bot (login/password, no Firebase) and falls back to
// login if the account already exists. Sets c.token on success.
func (c *Client) EnsureAuth(ctx context.Context, login, password string) error {
	body := map[string]string{"login": login, "password": password}

	var out authResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/register", body, &out)
	if err != nil {
		// Already registered (or any register failure) → try login.
		if loginErr := c.do(ctx, http.MethodPost, "/api/v1/auth/login", body, &out); loginErr != nil {
			return fmt.Errorf("register failed (%v) and login failed (%w)", err, loginErr)
		}
	}
	if out.SessionToken == "" {
		return fmt.Errorf("auth succeeded but no session_token returned")
	}
	c.token = out.SessionToken
	return nil
}

func (c *Client) GetPlayer(ctx context.Context) (player.Player, error) {
	var p player.Player
	err := c.do(ctx, http.MethodGet, "/api/v1/player", nil, &p)
	return p, err
}

// UpdatePosition moves the bot. The server runs anti-cheat speed validation
// (W-03): consecutive positions must be reachable under MAX_SPEED_KMH. The
// agent loop steps gradually to stay under the cap.
func (c *Client) UpdatePosition(ctx context.Context, lat, lng float64) error {
	req := player.UpdatePositionRequest{Lat: lat, Lng: lng}
	return c.do(ctx, http.MethodPatch, "/api/v1/player/position", req, nil)
}

// GetCells returns the cells around a point (radius_km <= 2.0).
func (c *Client) GetCells(ctx context.Context, lat, lng, radiusKm float64) ([]cell.CellDTO, error) {
	path := fmt.Sprintf("/api/v1/map/cells?lat=%f&lng=%f&radius_km=%f", lat, lng, radiusKm)
	var out struct {
		Cells []cell.CellDTO `json:"cells"`
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out.Cells, err
}

// PlaceTower builds a tower. Empty cellID = "at my current location": the server
// picks the cell nearest the bot's position, so the bot only needs to walk close.
func (c *Client) PlaceTower(ctx context.Context, cellID string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/towers", tower.PlaceRequest{CellID: cellID}, nil)
}

// Lockpick starts a stealth capture of an enemy tower.
func (c *Client) Lockpick(ctx context.Context, towerID string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/towers/"+towerID+"/capture/lockpick", nil, nil)
}

// --- Army / squads / battle -------------------------------------------------

// ListUnits returns the bot's army.
func (c *Client) ListUnits(ctx context.Context) ([]unit.Unit, error) {
	var out struct {
		Units []unit.Unit `json:"units"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/units", nil, &out)
	return out.Units, err
}

// RecruitUnit recruits one unit of the given type (fighter|engineer|scout|medic).
func (c *Client) RecruitUnit(ctx context.Context, unitType string) (unit.Unit, error) {
	var out struct {
		Unit unit.Unit `json:"unit"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/units", unit.RecruitRequest{Type: unitType}, &out)
	return out.Unit, err
}

// ListSquads returns the bot's squads.
func (c *Client) ListSquads(ctx context.Context) ([]squad.Squad, error) {
	var out struct {
		Squads []squad.Squad `json:"squads"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/squads", nil, &out)
	return out.Squads, err
}

// CreateSquad forms a squad from the given unit IDs.
func (c *Client) CreateSquad(ctx context.Context, name string, unitIDs []string) (squad.Squad, error) {
	var out struct {
		Squad squad.Squad `json:"squad"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/squads",
		squad.CreateRequest{Name: name, UnitIDs: unitIDs}, &out)
	return out.Squad, err
}

// StartBattle initiates a battle (target_type "rift"|"hive"|"tutorial").
func (c *Client) StartBattle(ctx context.Context, squadID, targetType, targetID string) (battle.Snapshot, error) {
	var snap battle.Snapshot
	err := c.do(ctx, http.MethodPost, "/api/v1/battles/start",
		battle.StartRequest{SquadID: squadID, TargetType: targetType, TargetID: targetID}, &snap)
	return snap, err
}

// BattleAction executes one round with a tactic (attack|defend|energy|retreat).
func (c *Client) BattleAction(ctx context.Context, battleID, tactic string) (battle.Snapshot, error) {
	var snap battle.Snapshot
	err := c.do(ctx, http.MethodPost, "/api/v1/battles/"+battleID+"/action",
		battle.ActionRequest{Tactic: tactic}, &snap)
	return snap, err
}

// --- Faction / PvP ----------------------------------------------------------

// ChooseFaction declares allegiance ("human"|"symbiont"). Requires prior Contact
// server-side, so this returns an error (not_contacted) until that's satisfied.
func (c *Client) ChooseFaction(ctx context.Context, side string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/faction/choose",
		faction.ChooseRequest{Side: side}, nil)
}

// PvpTargets lists enemy-domed cells near a point (symbiont breach candidates).
func (c *Client) PvpTargets(ctx context.Context, lat, lng float64) ([]pvp.Target, error) {
	path := fmt.Sprintf("/api/v1/pvp/targets?lat=%f&lng=%f", lat, lng)
	var out struct {
		Targets []pvp.Target `json:"targets"`
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out.Targets, err
}

// PvpBreach punctures an enemy dome on the given cell, opening a rift there.
func (c *Client) PvpBreach(ctx context.Context, cellID string) (pvp.BreachResult, error) {
	var res pvp.BreachResult
	err := c.do(ctx, http.MethodPost, "/api/v1/pvp/breach", pvp.BreachRequest{CellID: cellID}, &res)
	return res, err
}

// do performs an authenticated JSON request and decodes into out (may be nil).
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &apiError{Status: resp.StatusCode, Body: string(raw)}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode %s %s: %w", method, path, err)
		}
	}
	return nil
}
