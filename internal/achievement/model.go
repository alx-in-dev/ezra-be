package achievement

// Metric is the player stat an achievement measures. All metrics are derived
// live from existing tables (no separate stat-tracking infra), so achievements
// stay correct even for actions that predate this system.
type Metric string

const (
	MetricBeacons     Metric = "beacons"      // powered or not: total towers owned
	MetricCoreLevel   Metric = "core_level"   // current Core level
	MetricBattlesWon  Metric = "battles_won"  // resolved_victory battles as attacker
	MetricSpectra     Metric = "spectra"      // distinct resonance spectra collected
	MetricPlayerLevel Metric = "player_level" // account level
)

// Definition is one achievement: reach Threshold of Metric to unlock it and earn
// RewardCrystals (one-time). Order in Catalog is the display order.
type Definition struct {
	ID             string
	Title          string
	Description    string
	Metric         Metric
	Threshold      int
	RewardCrystals int
}

// Catalog is the full achievement set (MVP). Thresholds are deliberately spread
// so there's always a near-term goal. Rewards are modest crystals — a soft
// drip, not a power source (anti-P2W: crystals buy convenience/cosmetics).
var Catalog = []Definition{
	{"first_beacon", "Первый маяк", "Поставьте свой первый маяк", MetricBeacons, 1, 10},
	{"network_10", "Растущая сеть", "Владейте 10 маяками", MetricBeacons, 10, 15},
	{"network_25", "Архитектор сети", "Владейте 25 маяками", MetricBeacons, 25, 25},
	{"core_3", "Усиленное ядро", "Прокачайте Ядро до 3 уровня", MetricCoreLevel, 3, 15},
	{"core_5", "Предел ядра", "Прокачайте Ядро до 5 уровня", MetricCoreLevel, 5, 30},
	{"battle_1", "Первая победа", "Выиграйте первый бой", MetricBattlesWon, 1, 10},
	{"battle_10", "Закалённый в боях", "Выиграйте 10 боёв", MetricBattlesWon, 10, 20},
	{"battle_50", "Ветеран", "Выиграйте 50 боёв", MetricBattlesWon, 50, 40},
	{"spectra_3", "Собиратель спектров", "Соберите 3 разных спектра резонанса", MetricSpectra, 3, 15},
	{"spectra_all", "Полный резонанс", "Соберите все спектры резонанса", MetricSpectra, 5, 50},
	{"level_5", "Опытный", "Достигните 5 уровня", MetricPlayerLevel, 5, 15},
	{"level_10", "Мастер", "Достигните 10 уровня", MetricPlayerLevel, 10, 30},
}

// Metrics is a snapshot of every measured stat for a player.
type Metrics struct {
	Beacons     int
	CoreLevel   int
	BattlesWon  int
	Spectra     int
	PlayerLevel int
}

func (m Metrics) value(metric Metric) int {
	switch metric {
	case MetricBeacons:
		return m.Beacons
	case MetricCoreLevel:
		return m.CoreLevel
	case MetricBattlesWon:
		return m.BattlesWon
	case MetricSpectra:
		return m.Spectra
	case MetricPlayerLevel:
		return m.PlayerLevel
	default:
		return 0
	}
}

// Status is one achievement's client-facing state.
type Status struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Threshold      int    `json:"threshold"`
	Progress       int    `json:"progress"`
	RewardCrystals int    `json:"reward_crystals"`
	Unlocked       bool   `json:"unlocked"`
}
