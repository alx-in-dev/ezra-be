// Command bot runs a swarm of debug bots that play Ezra as symbiont players:
// they authenticate (login/password, no Firebase), move around the map under
// the anti-cheat speed cap, and take real player actions (build beacons,
// capture enemy towers). Point it at a server and let it run alongside your own
// real accounts to exercise multiplayer interactions.
//
// Usage:
//
//	go run ./cmd/bot
//	BOT_COUNT=5 BOT_BASE_URL=http://localhost:8080 go run ./cmd/bot
//	BOT_BRAIN=llm OPENROUTER_API_KEY=sk-or-... go run ./cmd/bot
//
// See config.go for every BOT_* / OPENROUTER_* knob.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

func main() {
	cfg := LoadConfig()

	// Groups come from the interactive launcher (`bot tui`) or, by default, from
	// the BOT_* env (a single faction group — backward compatible).
	var groups []Group
	if isTUI() {
		groups = runLauncher(&cfg)
	} else {
		groups = []Group{{Faction: cfg.Faction, Prefix: cfg.Prefix, Count: cfg.Count}}
	}

	level := slog.LevelInfo
	if cfg.LogLevelDebug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	slog.Info("starting debug bot swarm",
		"server", cfg.BaseURL, "brain", brainLabel(&cfg),
		"center", fmt.Sprintf("%.5f,%.5f", cfg.CenterLat, cfg.CenterLng))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	for _, g := range groups {
		gcfg := cfg // per-group copy so each agent gets its faction
		gcfg.Faction = g.Faction
		for i := 0; i < g.Count; i++ {
			login := fmt.Sprintf("%s%d", g.Prefix, i+1)
			lat, lng := scatter(cfg.CenterLat, cfg.CenterLng, cfg.SpawnRadiusM, i, g.Count)
			agent := NewAgent(login, gcfg, newBrain(gcfg), lat, lng)

			wg.Add(1)
			go func() {
				defer wg.Done()
				agent.Run(ctx)
			}()
		}
	}

	wg.Wait()
	slog.Info("all bots stopped")
}

// isTUI reports whether the bot was invoked in interactive launcher mode
// (`bot tui` / `bot -tui`).
func isTUI() bool {
	for _, a := range os.Args[1:] {
		if a == "tui" || a == "-tui" || a == "--tui" {
			return true
		}
	}
	return false
}

// newBrain builds a fresh brain per bot so they don't share mutable state.
func newBrain(cfg Config) Brain {
	if cfg.Brain == "llm" {
		// A key is required for OpenRouter, but a local OpenAI-compatible server
		// (Ollama) needs none — only insist on a key when targeting OpenRouter.
		isOpenRouter := strings.Contains(cfg.LLMBaseURL, "openrouter.ai")
		if isOpenRouter && cfg.OpenRouterKey == "" {
			slog.Warn("BOT_BRAIN=llm but OPENROUTER_API_KEY is empty; falling back to scripted")
			return &ScriptedBrain{}
		}
		return NewLLMBrain(cfg.OpenRouterKey, cfg.OpenRouterModel, cfg.LLMBaseURL, cfg.LLMReasoning)
	}
	return &ScriptedBrain{}
}

// scatter spreads bots evenly on a circle around the center so they don't all
// spawn on the same cell. radiusM is converted to degrees (lat ~111.32 km/deg,
// lng scaled by cos(lat)).
func scatter(centerLat, centerLng, radiusM float64, idx, total int) (lat, lng float64) {
	if total <= 1 || radiusM <= 0 {
		return centerLat, centerLng
	}
	angle := 2 * math.Pi * float64(idx) / float64(total)
	dLat := (radiusM * math.Cos(angle)) / 111320.0
	dLng := (radiusM * math.Sin(angle)) / (111320.0 * math.Cos(centerLat*math.Pi/180))
	return centerLat + dLat, centerLng + dLng
}
