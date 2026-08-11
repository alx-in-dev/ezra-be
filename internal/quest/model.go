package quest

import "time"

// DailyQuest represents a single daily quest assigned to a player.
type DailyQuest struct {
	ID          string    `json:"id" db:"id"`
	PlayerID    string    `json:"player_id" db:"player_id"`
	Type        string    `json:"type" db:"type"`
	Description string    `json:"description" db:"description"`
	Progress    int       `json:"progress" db:"progress"`
	Target      int       `json:"target" db:"target"`
	Reward      Reward    `json:"reward"` // JSONB
	Date        time.Time `json:"date" db:"date"`
	Claimed     bool      `json:"claimed" db:"claimed"`
}

// Reward describes quest/streak reward resources.
type Reward struct {
	Energy    int `json:"energy"`
	Materials int `json:"materials"`
	Crystals  int `json:"crystals,omitempty"`
}

// Streak tracks a player's daily check-in streak.
type Streak struct {
	PlayerID    string     `json:"player_id" db:"player_id"`
	Days        int        `json:"days" db:"days"`
	LastCheckin *time.Time `json:"last_checkin" db:"last_checkin"`
}

// QuestTemplate defines a quest blueprint for daily generation.
type QuestTemplate struct {
	Type        string
	Description string
	Target      int
	Reward      Reward
}

// QuestTemplates is the pool of available daily quests.
//
// Reward values are picked from the canonical ranges in
// docs/feature/balance_tables.md "Daily quest rewards":
//   - Простое действие: 25-40⚡
//   - Среднее действие: 40-70⚡ + 5-10🔩
//   - Боевое / территориальное: 60-100⚡ + 10-20🔩
//
// MVP target totals: ~140-180⚡ and ~10-25🔩 per day.
// Current pack (one of each tier): 30 + 55 + 80 = 165⚡, 8 + 15 = 23🔩.
var QuestTemplates = []QuestTemplate{
	// Простое действие
	{"send_pet", "Отправить питомца", 1, Reward{Energy: 30}},
	// Среднее действие — постановка маяка
	{"place_beacon", "Поставить 1 маяк", 1, Reward{Energy: 55, Materials: 8}},
	// Боевое / территориальное
	{"win_3_battles", "Победить в 3 боях", 3, Reward{Energy: 80, Materials: 15}},
}

// StreakBonuses maps streak day thresholds to bonus rewards.
//
// Values from docs/feature/balance_tables.md "Streak rewards":
//   - 3   -> +50⚡
//   - 7   -> +150⚡ + 20💎
//   - 14  -> +250⚡ + 40💎 + редкий юнит  (rare-unit reward deferred — Reward
//                                          struct lacks an item field; tracked
//                                          in ai/tasks.md Stage 2.1)
//   - 30  -> +400⚡ + 100💎 + косметика   (cosmetic reward deferred — same)
var StreakBonuses = map[int]Reward{
	3:  {Energy: 50},
	7:  {Energy: 150, Crystals: 20},
	14: {Energy: 250, Crystals: 40},
	30: {Energy: 400, Crystals: 100},
}
