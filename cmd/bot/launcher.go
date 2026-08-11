package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Group is one cohort of bots: a faction with a login prefix and a count. The
// launcher (or env defaults) produces a list of these for main to spawn.
type Group struct {
	Faction string
	Prefix  string
	Count   int
}

var stdin = bufio.NewScanner(os.Stdin)

// runLauncher walks an interactive console menu, mutating cfg in place and
// returning the bot groups to spawn. No deps — just numbered prompts.
func runLauncher(cfg *Config) []Group {
	fmt.Println("\n  ⎔ Ezra debug-bot launcher")
	fmt.Println("  ─────────────────────────")

	// Offer to reuse the previous run's config — one keystroke, no re-entry.
	if st, ok := loadState(); ok {
		fmt.Printf("\n  Last config: %s | %s | %.4f,%.4f | %s\n",
			st.BaseURL, st.brainLabel(), st.CenterLat, st.CenterLng, st.groupLabel())
		if askYesNo("Reuse last config", true) {
			st.applyTo(cfg)
			return st.Groups
		}
	}

	// Server: pick from a menu so there's nothing to export/type by hand.
	switch askChoice("Game server", []string{
		"Local docker (http://localhost:8081)  [recommended]",
		"Prod / test box (http://94.228.120.50:8081)",
		"Custom URL",
	}, 1) {
	case 1:
		cfg.BaseURL = "http://localhost:8081"
	case 2:
		cfg.BaseURL = "http://94.228.120.50:8081"
	case 3:
		cfg.BaseURL = askText("Game server URL", cfg.BaseURL)
	}

	// Region.
	switch askChoice("Spawn region", []string{
		"Samara — has cells (53.13, 50.15)  [recommended]",
		"Kaliningrad — cactus territory (54.737, 20.555)",
		"Moscow center (55.7512, 37.6184)",
		"Custom lat/lng",
	}, 1) {
	case 1:
		cfg.CenterLat, cfg.CenterLng = 53.13, 50.15
	case 2:
		cfg.CenterLat, cfg.CenterLng = 54.737105, 20.555084
	case 3:
		cfg.CenterLat, cfg.CenterLng = 55.751244, 37.618423
	case 4:
		cfg.CenterLat = askFloat("Center lat", cfg.CenterLat)
		cfg.CenterLng = askFloat("Center lng", cfg.CenterLng)
	}
	cfg.SpawnRadiusM = askFloat("Spawn radius (m)", 250)

	// Brain.
	switch askChoice("Brain", []string{
		"Scripted — fast, deterministic, free  [recommended]",
		"LLM via OpenRouter (needs OPENROUTER_API_KEY)",
		"LLM via local Ollama (192.168.0.100)",
	}, 1) {
	case 1:
		cfg.Brain = "scripted"
	case 2:
		cfg.Brain = "llm"
		cfg.LLMBaseURL = "https://openrouter.ai/api/v1"
		cfg.OpenRouterKey = os.Getenv("OPENROUTER_API_KEY")
		if cfg.OpenRouterKey == "" {
			fmt.Println("  ! OPENROUTER_API_KEY is empty — bots will fall back to scripted.")
		}
		cfg.OpenRouterModel = askText("Model", "openai/gpt-oss-20b")
		if strings.Contains(cfg.OpenRouterModel, "gpt-oss") {
			cfg.LLMReasoning = "low" // reasoning model needs the throttle
		}
	case 3:
		cfg.Brain = "llm"
		cfg.LLMBaseURL = askText("Ollama base URL", "http://192.168.0.100:11434/v1")
		cfg.OpenRouterKey = ""
		cfg.OpenRouterModel = askText("Model", "llama3:8b")
	}

	if cfg.Brain == "llm" {
		// LLM inference is slow under GPU contention — pace it slower by default.
		cfg.Tick = msToDuration(askInt("Tick (ms)", 3500))
	} else {
		cfg.Tick = msToDuration(askInt("Tick (ms)", 2500))
	}

	// Composition.
	var groups []Group
	switch askChoice("Composition", []string{
		"Two factions (humans vs symbionts)  [recommended]",
		"Humans only",
		"Symbionts only",
	}, 1) {
	case 1:
		h := askInt("How many humans", 5)
		s := askInt("How many symbionts", 5)
		if h > 0 {
			groups = append(groups, Group{Faction: "human", Prefix: "hbot", Count: h})
		}
		if s > 0 {
			groups = append(groups, Group{Faction: "symbiont", Prefix: "sbot", Count: s})
		}
	case 2:
		groups = append(groups, Group{Faction: "human", Prefix: "hbot", Count: askInt("How many humans", 5)})
	case 3:
		groups = append(groups, Group{Faction: "symbiont", Prefix: "sbot", Count: askInt("How many symbionts", 5)})
	}

	cfg.LogLevelDebug = askYesNo("Verbose (debug) logs", false)

	// Summary.
	total := 0
	for _, g := range groups {
		total += g.Count
	}
	fmt.Printf("\n  ▶ %d bots | brain=%s | region=%.4f,%.4f | tick=%v\n",
		total, brainLabel(cfg), cfg.CenterLat, cfg.CenterLng, cfg.Tick)
	for _, g := range groups {
		fmt.Printf("      %d × %s\n", g.Count, g.Faction)
	}
	if !askYesNo("Launch", true) {
		fmt.Println("  aborted.")
		os.Exit(0)
	}
	saveState(cfg, groups) // remember for next time
	return groups
}

// --- persisted config (so the TUI doesn't ask everything again) -------------

type launcherState struct {
	BaseURL      string  `json:"base_url"`
	CenterLat    float64 `json:"center_lat"`
	CenterLng    float64 `json:"center_lng"`
	SpawnRadiusM float64 `json:"spawn_radius_m"`
	Brain        string  `json:"brain"`
	LLMBaseURL   string  `json:"llm_base_url"`
	Model        string  `json:"model"`
	Reasoning    string  `json:"reasoning"`
	TickMS       int     `json:"tick_ms"`
	Debug        bool    `json:"debug"`
	Groups       []Group `json:"groups"`
}

func statePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ezra-bot-tui.json")
}

func loadState() (*launcherState, bool) {
	p := statePath()
	if p == "" {
		return nil, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	var st launcherState
	if json.Unmarshal(data, &st) != nil || len(st.Groups) == 0 {
		return nil, false
	}
	return &st, true
}

func saveState(cfg *Config, groups []Group) {
	p := statePath()
	if p == "" {
		return
	}
	st := launcherState{
		BaseURL: cfg.BaseURL, CenterLat: cfg.CenterLat, CenterLng: cfg.CenterLng,
		SpawnRadiusM: cfg.SpawnRadiusM, Brain: cfg.Brain, LLMBaseURL: cfg.LLMBaseURL,
		Model: cfg.OpenRouterModel, Reasoning: cfg.LLMReasoning,
		TickMS: int(cfg.Tick.Milliseconds()), Debug: cfg.LogLevelDebug, Groups: groups,
	}
	if data, err := json.MarshalIndent(st, "", "  "); err == nil {
		_ = os.WriteFile(p, data, 0o644)
	}
}

// applyTo restores a saved state into cfg. The API key is never persisted — it
// is re-read from the environment when reusing an OpenRouter config.
func (st *launcherState) applyTo(cfg *Config) {
	cfg.BaseURL = st.BaseURL
	cfg.CenterLat, cfg.CenterLng = st.CenterLat, st.CenterLng
	cfg.SpawnRadiusM = st.SpawnRadiusM
	cfg.Brain = st.Brain
	cfg.LLMBaseURL = st.LLMBaseURL
	cfg.OpenRouterModel = st.Model
	cfg.LLMReasoning = st.Reasoning
	cfg.Tick = msToDuration(st.TickMS)
	cfg.LogLevelDebug = st.Debug
	if cfg.Brain == "llm" && strings.Contains(cfg.LLMBaseURL, "openrouter.ai") {
		cfg.OpenRouterKey = os.Getenv("OPENROUTER_API_KEY")
	}
}

func (st *launcherState) brainLabel() string {
	if st.Brain == "llm" {
		return "llm:" + st.Model
	}
	return "scripted"
}

func (st *launcherState) groupLabel() string {
	parts := make([]string, 0, len(st.Groups))
	for _, g := range st.Groups {
		parts = append(parts, fmt.Sprintf("%d %s", g.Count, g.Faction))
	}
	return strings.Join(parts, " + ")
}

func brainLabel(cfg *Config) string {
	if cfg.Brain == "llm" {
		return "llm:" + cfg.OpenRouterModel
	}
	return "scripted"
}

// --- prompt helpers ---------------------------------------------------------

func prompt(label, def string) string {
	if def != "" {
		fmt.Printf("  %s [%s]: ", label, def)
	} else {
		fmt.Printf("  %s: ", label)
	}
	if !stdin.Scan() {
		return def
	}
	return strings.TrimSpace(stdin.Text())
}

func askText(label, def string) string {
	if v := prompt(label, def); v != "" {
		return v
	}
	return def
}

func askInt(label string, def int) int {
	for {
		v := prompt(label, strconv.Itoa(def))
		if v == "" {
			return def
		}
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
		fmt.Println("    enter a non-negative number")
	}
}

func askFloat(label string, def float64) float64 {
	v := prompt(label, strconv.FormatFloat(def, 'f', -1, 64))
	if v == "" {
		return def
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	return def
}

func askYesNo(label string, def bool) bool {
	d := "y/N"
	if def {
		d = "Y/n"
	}
	v := strings.ToLower(prompt(label, d))
	if v == "" || v == d {
		return def
	}
	return v == "y" || v == "yes"
}

// askChoice prints a numbered menu and returns the 1-based selection.
func askChoice(label string, options []string, def int) int {
	fmt.Printf("\n  %s:\n", label)
	for i, o := range options {
		fmt.Printf("    %d) %s\n", i+1, o)
	}
	for {
		v := prompt("choice", strconv.Itoa(def))
		if v == "" {
			return def
		}
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= len(options) {
			return n
		}
		fmt.Printf("    enter 1..%d\n", len(options))
	}
}
