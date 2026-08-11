package main

import (
	"os"
	"strconv"
	"time"
)

// Config controls the debug bot swarm. Everything is overridable via env so you
// can point it at the prod box or localhost without recompiling.
type Config struct {
	BaseURL string // server base, e.g. http://localhost:8080

	Count    int    // how many bots to spawn
	Prefix   string // login prefix; bot N logs in as "<prefix>N"
	Password string // shared password for all bots (legacy login/password auth)

	// Spawn area. Bots are scattered within SpawnRadiusM of the center.
	CenterLat    float64
	CenterLng    float64
	SpawnRadiusM float64

	Tick        time.Duration // decision/move cadence per bot
	WalkSpeedMS float64       // metres per tick-second; keep under the server's MAX_SPEED_KMH

	Brain string // "scripted" | "llm"

	// Faction side: "" (none), "human", or "symbiont". Symbionts attempt Dome
	// Breach against enemy domes; the agent tries POST /faction/choose at start.
	Faction string

	// LLM brain. Only used when Brain == "llm". BaseURL defaults to OpenRouter
	// but accepts any OpenAI-compatible endpoint (e.g. a local Ollama at
	// http://192.168.0.100:11434/v1). Key may be empty for keyless local servers.
	OpenRouterKey   string
	OpenRouterModel string
	LLMBaseURL      string
	// LLMReasoning sets the reasoning effort ("low"/"medium"/"high") for reasoning
	// models like gpt-oss. Empty (default) omits the param — non-reasoning models
	// (qwen, llama, mistral) reject it with a 400.
	LLMReasoning string

	// LogLevelDebug enables debug logging (moves/decisions). Set from
	// BOT_LOG_LEVEL=debug or by the interactive launcher.
	LogLevelDebug bool
}

func LoadConfig() Config {
	return Config{
		BaseURL: getenv("BOT_BASE_URL", "http://localhost:8080"),

		Count:    getenvInt("BOT_COUNT", 3),
		Prefix:   getenv("BOT_PREFIX", "dbgbot"),
		Password: getenv("BOT_PASSWORD", "botpass123"),

		CenterLat:    getenvFloat("BOT_CENTER_LAT", 55.751244), // Moscow center by default
		CenterLng:    getenvFloat("BOT_CENTER_LNG", 37.618423),
		SpawnRadiusM: getenvFloat("BOT_SPAWN_RADIUS_M", 300),

		Tick:        time.Duration(getenvInt("BOT_TICK_MS", 2500)) * time.Millisecond,
		WalkSpeedMS: getenvFloat("BOT_WALK_SPEED_MS", 8), // ~28.8 km/h, under the 50 km/h anti-cheat cap

		Brain:   getenv("BOT_BRAIN", "scripted"),
		Faction: getenv("BOT_FACTION", ""),

		OpenRouterKey:   os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel: getenv("OPENROUTER_MODEL", "openai/gpt-oss-20b"),
		LLMBaseURL:      getenv("LLM_BASE_URL", "https://openrouter.ai/api/v1"),
		LLMReasoning:    os.Getenv("LLM_REASONING_EFFORT"),
		LogLevelDebug:   getenv("BOT_LOG_LEVEL", "") == "debug",
	}
}

func msToDuration(ms int) time.Duration { return time.Duration(ms) * time.Millisecond }

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
