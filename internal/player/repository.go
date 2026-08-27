package player

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ezra-game/server/internal/canon"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrPlayerNotFound = errors.New("player not found")

type Repository interface {
	Create(ctx context.Context, p *Player) error
	GetByID(ctx context.Context, id string) (*Player, error)
	GetAll(ctx context.Context) ([]Player, error)
	GetByFirebaseUID(ctx context.Context, uid string) (*Player, error)
	GetByLogin(ctx context.Context, login string) (*Player, error)
	CreateWithLogin(ctx context.Context, p *Player) error
	Update(ctx context.Context, p *Player) error
	UpdatePosition(ctx context.Context, id string, lat, lng float64) error
	UpdateLastActive(ctx context.Context, id string) error
	// SetQuickStartHuman flags a player for the onboarding quick-start Human
	// path (docs/feature/onboarding_quick_start.md). Dedicated column, outside
	// Update's generic SET list — same pattern as crystals/resonance.
	SetQuickStartHuman(ctx context.Context, id string) error
}

type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) *PgRepository {
	return &PgRepository{db: db}
}

func (r *PgRepository) Create(ctx context.Context, p *Player) error {
	skillsJSON, _ := json.Marshal(p.Skills)
	posJSON, _ := json.Marshal(p.Position)

	query := `
		INSERT INTO players (firebase_uid, username, username_is_custom, onboarding_step, starter_beacon_available, level, xp, energy, materials, army_limit, skills, position)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, last_active`

	return r.db.QueryRow(ctx, query,
		p.FirebaseUID, p.Username, p.UsernameIsCustom, p.OnboardingStep, p.StarterBeaconAvailable,
		p.Level, p.XP, p.Energy, p.Materials, p.ArmyLimit, skillsJSON, posJSON,
	).Scan(&p.ID, &p.CreatedAt, &p.LastActive)
}

func (r *PgRepository) GetByID(ctx context.Context, id string) (*Player, error) {
	return r.getOne(ctx, "SELECT id, firebase_uid, login, password_hash, username, username_is_custom, onboarding_step, starter_beacon_available, level, xp, energy, materials, army_limit, crystals, storage_bonus_energy, pet_slots, symbiont_resonance, quick_start_human, skills, position, last_active, created_at FROM players WHERE id = $1", id)
}

// CommanderPoints returns the player's Commander skill points (roster.CommanderReader,
// T-846). Reads the skills JSON directly to avoid loading the full row.
func (r *PgRepository) CommanderPoints(ctx context.Context, id string) (int, error) {
	p, err := r.GetByID(ctx, id)
	if err != nil || p == nil {
		return 0, err
	}
	return p.Skills.Commander, nil
}

func (r *PgRepository) GetAll(ctx context.Context) ([]Player, error) {
	rows, err := r.db.Query(ctx, "SELECT id, firebase_uid, login, password_hash, username, username_is_custom, onboarding_step, starter_beacon_available, level, xp, energy, materials, army_limit, crystals, storage_bonus_energy, pet_slots, symbiont_resonance, quick_start_human, skills, position, last_active, created_at FROM players")
	if err != nil {
		return nil, fmt.Errorf("list players: %w", err)
	}
	defer rows.Close()

	var players []Player
	for rows.Next() {
		var p Player
		var skillsJSON, posJSON []byte
		var firebaseUID, login, passwordHash *string
		if err := rows.Scan(
			&p.ID, &firebaseUID, &login, &passwordHash, &p.Username, &p.UsernameIsCustom, &p.OnboardingStep, &p.StarterBeaconAvailable,
			&p.Level, &p.XP, &p.Energy, &p.Materials, &p.ArmyLimit, &p.Crystals, &p.StorageBonusEnergy, &p.PetSlots, &p.SymbiontResonance, &p.QuickStartHuman, &skillsJSON, &posJSON, &p.LastActive, &p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan player: %w", err)
		}
		if firebaseUID != nil {
			p.FirebaseUID = *firebaseUID
		}
		if login != nil {
			p.Login = *login
		}
		if passwordHash != nil {
			p.PasswordHash = *passwordHash
		}
		json.Unmarshal(skillsJSON, &p.Skills)
		json.Unmarshal(posJSON, &p.Position)
		players = append(players, p)
	}
	return players, rows.Err()
}

func (r *PgRepository) GetByFirebaseUID(ctx context.Context, uid string) (*Player, error) {
	return r.getOne(ctx, "SELECT id, firebase_uid, login, password_hash, username, username_is_custom, onboarding_step, starter_beacon_available, level, xp, energy, materials, army_limit, crystals, storage_bonus_energy, pet_slots, symbiont_resonance, quick_start_human, skills, position, last_active, created_at FROM players WHERE firebase_uid = $1", uid)
}

func (r *PgRepository) GetByLogin(ctx context.Context, login string) (*Player, error) {
	return r.getOne(ctx, "SELECT id, firebase_uid, login, password_hash, username, username_is_custom, onboarding_step, starter_beacon_available, level, xp, energy, materials, army_limit, crystals, storage_bonus_energy, pet_slots, symbiont_resonance, quick_start_human, skills, position, last_active, created_at FROM players WHERE login = $1", login)
}

func (r *PgRepository) CreateWithLogin(ctx context.Context, p *Player) error {
	skillsJSON, _ := json.Marshal(p.Skills)
	posJSON, _ := json.Marshal(p.Position)

	query := `
		INSERT INTO players (login, password_hash, username, username_is_custom, onboarding_step, starter_beacon_available, level, xp, energy, materials, army_limit, skills, position)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at, last_active`

	return r.db.QueryRow(ctx, query,
		p.Login, p.PasswordHash, p.Username, p.UsernameIsCustom, p.OnboardingStep, p.StarterBeaconAvailable,
		p.Level, p.XP, p.Energy, p.Materials, p.ArmyLimit, skillsJSON, posJSON,
	).Scan(&p.ID, &p.CreatedAt, &p.LastActive)
}

// getOne scans a single player row handling JSONB columns.
func (r *PgRepository) getOne(ctx context.Context, query string, args ...any) (*Player, error) {
	var p Player
	var skillsJSON, posJSON []byte
	var firebaseUID, login, passwordHash *string

	err := r.db.QueryRow(ctx, query, args...).Scan(
		&p.ID, &firebaseUID, &login, &passwordHash, &p.Username, &p.UsernameIsCustom, &p.OnboardingStep, &p.StarterBeaconAvailable,
		&p.Level, &p.XP, &p.Energy, &p.Materials, &p.ArmyLimit, &p.Crystals, &p.StorageBonusEnergy, &p.PetSlots,
		&p.SymbiontResonance, &p.QuickStartHuman,
		&skillsJSON, &posJSON,
		&p.LastActive, &p.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlayerNotFound
		}
		return nil, fmt.Errorf("get player: %w", err)
	}
	if firebaseUID != nil {
		p.FirebaseUID = *firebaseUID
	}
	if login != nil {
		p.Login = *login
	}
	if passwordHash != nil {
		p.PasswordHash = *passwordHash
	}

	json.Unmarshal(skillsJSON, &p.Skills)
	json.Unmarshal(posJSON, &p.Position)
	return &p, nil
}

func (r *PgRepository) Update(ctx context.Context, p *Player) error {
	skillsJSON, _ := json.Marshal(p.Skills)
	query := `UPDATE players SET username=$1, username_is_custom=$2, onboarding_step=$3, starter_beacon_available=$4, level=$5, xp=$6, energy=$7, materials=$8, army_limit=$9, skills=$10, last_active=NOW() WHERE id=$11`
	_, err := r.db.Exec(ctx, query, p.Username, p.UsernameIsCustom, p.OnboardingStep, p.StarterBeaconAvailable, p.Level, p.XP, p.Energy, p.Materials, p.ArmyLimit, skillsJSON, p.ID)
	return err
}

func (r *PgRepository) UpdatePosition(ctx context.Context, id string, lat, lng float64) error {
	pos := Position{Lat: lat, Lng: lng, UpdatedAt: time.Now()}
	posJSON, _ := json.Marshal(pos)
	query := `UPDATE players SET position=$1, last_active=NOW() WHERE id=$2`
	_, err := r.db.Exec(ctx, query, posJSON, id)
	return err
}

func (r *PgRepository) UpdateLastActive(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE players SET last_active=NOW() WHERE id=$1`, id)
	return err
}

// IncrementArmyLimit atomically raises a player's army cap (shop expansion).
func (r *PgRepository) IncrementArmyLimit(ctx context.Context, id string, delta int) error {
	_, err := r.db.Exec(ctx, `UPDATE players SET army_limit = army_limit + $1 WHERE id = $2`, delta, id)
	return err
}

// IncrementEnergyStorageBonus atomically raises a player's energy cap bonus
// (shop expansion: storage_500 / storage_2000).
func (r *PgRepository) IncrementEnergyStorageBonus(ctx context.Context, id string, delta int) error {
	_, err := r.db.Exec(ctx, `UPDATE players SET storage_bonus_energy = storage_bonus_energy + $1 WHERE id = $2`, delta, id)
	return err
}

// IncrementPetSlots atomically raises a player's pet capacity (shop expansion).
func (r *PgRepository) IncrementPetSlots(ctx context.Context, id string, delta int) error {
	_, err := r.db.Exec(ctx, `UPDATE players SET pet_slots = pet_slots + $1 WHERE id = $2`, delta, id)
	return err
}

// IncrementCrystals atomically adjusts a player's crystals (faction-war season
// rewards and other grants).
func (r *PgRepository) IncrementCrystals(ctx context.Context, id string, delta int) error {
	_, err := r.db.Exec(ctx, `UPDATE players SET crystals = GREATEST(0, crystals + $1) WHERE id = $2`, delta, id)
	return err
}

// AddSymbiontResonance atomically adds delta to a player's portable Symbiont
// Resonance and returns the new total (#6 baseless progression). Floored at 0.
func (r *PgRepository) AddSymbiontResonance(ctx context.Context, id string, delta int) (int, error) {
	var total int
	err := r.db.QueryRow(ctx,
		`UPDATE players SET symbiont_resonance = GREATEST(0, symbiont_resonance + $1)
		 WHERE id = $2 RETURNING symbiont_resonance`, delta, id).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("add symbiont resonance: %w", err)
	}
	return total, nil
}

// TryDrainSymbiontEnergy drains `amount` energy from a player, but only if the
// last drain was more than intervalMin minutes ago (atomic throttle via
// symbiont_last_drain_at). Returns the drained amount (0 if throttled) and the
// resulting energy. Soft-drain under a hostile dome (#6 T-761).
func (r *PgRepository) TryDrainSymbiontEnergy(ctx context.Context, id string, amount, intervalMin int) (drained, energy int, err error) {
	if amount <= 0 {
		return 0, 0, nil
	}
	row := r.db.QueryRow(ctx, `
		UPDATE players
		   SET energy = GREATEST(0, energy - $2), symbiont_last_drain_at = now()
		 WHERE id = $1
		   AND (symbiont_last_drain_at IS NULL OR symbiont_last_drain_at < now() - make_interval(mins => $3))
		RETURNING energy`, id, amount, intervalMin)
	if scanErr := row.Scan(&energy); scanErr != nil {
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return 0, 0, nil // throttled — no drain this poll
		}
		return 0, 0, fmt.Errorf("drain symbiont energy: %w", scanErr)
	}
	return amount, energy, nil
}

// ActiveSymbiontIDs returns Symbiont-faction players active within the window
// — the population the server-side drain tick walks (#6 T-761).
func (r *PgRepository) ActiveSymbiontIDs(ctx context.Context, activeWithin time.Duration) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id FROM players p
		JOIN player_factions pf ON pf.player_id = p.id AND pf.faction = 'symbiont'
		WHERE p.last_active > now() - make_interval(mins => $1)`,
		int(activeWithin.Minutes()))
	if err != nil {
		return nil, fmt.Errorf("active symbionts: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SymbiontResonanceOf returns a player's portable Symbiont Resonance (#6).
func (r *PgRepository) SymbiontResonanceOf(ctx context.Context, id string) (int, error) {
	var n int
	if err := r.db.QueryRow(ctx,
		`SELECT symbiont_resonance FROM players WHERE id = $1`, id).Scan(&n); err != nil {
		return 0, fmt.Errorf("get symbiont resonance: %w", err)
	}
	return n, nil
}

// ResonanceProgressOf returns the player's Resonance Pool, Level and XP (R4-2).
func (r *PgRepository) ResonanceProgressOf(ctx context.Context, id string) (pool, level, xp int, err error) {
	if err = r.db.QueryRow(ctx,
		`SELECT resonance_pool, resonance_level, resonance_xp FROM players WHERE id = $1`, id,
	).Scan(&pool, &level, &xp); err != nil {
		return 0, 0, 0, fmt.Errorf("get resonance progress: %w", err)
	}
	return pool, level, xp, nil
}

// SetResonancePool stores the one-time human→Symbiont conversion value (R4-2),
// only if the pool hasn't been seeded yet (resonance_pool = 0). Returns the
// stored pool (existing value if already seeded — idempotent across re-switches).
func (r *PgRepository) SetResonancePool(ctx context.Context, id string, pool int) (int, error) {
	var stored int
	err := r.db.QueryRow(ctx,
		`UPDATE players SET resonance_pool = $2
		 WHERE id = $1 AND resonance_pool = 0
		 RETURNING resonance_pool`, id, pool).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		// Already seeded — return the existing value, don't overwrite.
		return r.resonancePoolOf(ctx, id)
	}
	if err != nil {
		return 0, fmt.Errorf("set resonance pool: %w", err)
	}
	return stored, nil
}

func (r *PgRepository) resonancePoolOf(ctx context.Context, id string) (int, error) {
	var p int
	if err := r.db.QueryRow(ctx, `SELECT resonance_pool FROM players WHERE id = $1`, id).Scan(&p); err != nil {
		return 0, fmt.Errorf("get resonance pool: %w", err)
	}
	return p, nil
}

// AddResonanceXP atomically adds delta Resonance XP, recomputes the Resonance
// Level from canon thresholds, persists both, and returns the new xp + level
// (R4-2). leveledUp reports whether the level advanced this call.
func (r *PgRepository) AddResonanceXP(ctx context.Context, id string, delta int) (xp, level int, leveledUp bool, err error) {
	var prevLevel int
	if err = r.db.QueryRow(ctx,
		`UPDATE players SET resonance_xp = GREATEST(0, resonance_xp + $1)
		 WHERE id = $2 RETURNING resonance_xp, resonance_level`, delta, id,
	).Scan(&xp, &prevLevel); err != nil {
		return 0, 0, false, fmt.Errorf("add resonance xp: %w", err)
	}
	level = canon.ResonanceLevelForXP(xp)
	if level != prevLevel {
		if _, err = r.db.Exec(ctx, `UPDATE players SET resonance_level = $2 WHERE id = $1`, id, level); err != nil {
			return 0, 0, false, fmt.Errorf("update resonance level: %w", err)
		}
	}
	return xp, level, level > prevLevel, nil
}

// SpendCrystals atomically deducts amount from a player's crystals, but only if
// the balance covers it. Returns ok=false (no deduction) when the player can't
// afford it — so callers reject a paid action without a separate balance
// read/race. amount <= 0 is a no-op that succeeds.
func (r *PgRepository) SpendCrystals(ctx context.Context, id string, amount int) (bool, error) {
	if amount <= 0 {
		return true, nil
	}
	tag, err := r.db.Exec(ctx,
		`UPDATE players SET crystals = crystals - $2 WHERE id = $1 AND crystals >= $2`, id, amount)
	if err != nil {
		return false, fmt.Errorf("spend crystals: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// SetExhaustion sets the player's N2 "Истощение" debuff window (spirit touch).
func (r *PgRepository) SetExhaustion(ctx context.Context, id string, until time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE players SET exhausted_until = $2 WHERE id = $1`, id, until)
	if err != nil {
		return fmt.Errorf("set exhaustion: %w", err)
	}
	return nil
}

// SetQuickStartHuman flags a player for the onboarding quick-start Human path
// (docs/feature/onboarding_quick_start.md).
func (r *PgRepository) SetQuickStartHuman(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE players SET quick_start_human = true WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("set quick start human: %w", err)
	}
	return nil
}
