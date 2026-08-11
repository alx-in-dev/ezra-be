package platform

import (
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"gopkg.in/yaml.v3"
)

// Balance holds all game balance parameters loaded from config/balance.yaml.
type Balance struct {
	Infection   InfectionBalance   `yaml:"infection"`
	Tower       TowerBalance       `yaml:"tower"`
	Battle      BattleBalance      `yaml:"battle"`
	Army        ArmyBalance        `yaml:"army"`
	Pet         PetBalance         `yaml:"pet"`
	Resources   ResourcesBalance   `yaml:"resources"`
	Progression ProgressionBalance `yaml:"progression"`
}

type InfectionBalance struct {
	BaseGrowthPerHour float64 `yaml:"base_growth_per_hour"`
	NeighborBonus     float64 `yaml:"neighbor_bonus"`
	RecalcIntervalMin int     `yaml:"recalc_interval_min"`
}

type TowerBalance struct {
	OverloadRadiusM  float64 `yaml:"overload_radius_m"`
	OverloadMaxCount int     `yaml:"overload_max_count"`
}

type BattleBalance struct {
	TimeoutSec            int     `yaml:"timeout_sec"`
	OverchargeMultiplier  float64 `yaml:"overcharge_multiplier"`
	OverchargeLossChance  float64 `yaml:"overcharge_loss_chance"`
}

type ArmyBalance struct {
	DecayRatePer24H float64 `yaml:"decay_rate_per_24h"`
	MaxDecay        float64 `yaml:"max_decay"`
	FreezeAfterDays int     `yaml:"freeze_after_days"`
}

type PetBalance struct {
	SearchDurationMin []int     `yaml:"search_duration_min"`
	RareChance        []float64 `yaml:"rare_chance"`
}

type ResourcesBalance struct {
	MaxEnergy    int `yaml:"max_energy"`
	MaxMaterials int `yaml:"max_materials"`
}

type ProgressionBalance struct {
	XPBase     int     `yaml:"xp_base"`
	XPExponent float64 `yaml:"xp_exponent"`
}

var (
	balanceMu   sync.RWMutex
	balanceData Balance
)

const defaultBalancePath = "config/balance.yaml"

// LoadBalance reads balance.yaml from disk.
func LoadBalance(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var b Balance
	if err := yaml.Unmarshal(data, &b); err != nil {
		return err
	}
	balanceMu.Lock()
	balanceData = b
	balanceMu.Unlock()
	slog.Info("balance config loaded", "path", path)
	return nil
}

// GetBalance returns a snapshot of current balance values.
func GetBalance() Balance {
	balanceMu.RLock()
	defer balanceMu.RUnlock()
	return balanceData
}

// WatchBalanceReload sets up SIGHUP handler for hot-reloading balance.yaml.
func WatchBalanceReload(path string) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	go func() {
		for range ch {
			slog.Info("SIGHUP received, reloading balance config")
			if err := LoadBalance(path); err != nil {
				slog.Error("failed to reload balance config", "error", err)
			}
		}
	}()
}
