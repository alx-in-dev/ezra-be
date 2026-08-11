package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ezra-game/server/pkg/geo"
)

// LLMBrain asks an LLM (via OpenRouter) for the next action. It implements the
// same Brain contract as ScriptedBrain, so the agent loop is unchanged — only
// BOT_BRAIN=llm selects it. Falls back to a scripted decision if the call fails,
// so a flaky network never freezes the swarm.
type LLMBrain struct {
	apiKey    string
	model     string
	baseURL   string // OpenAI-compatible base, e.g. OpenRouter or a local Ollama
	reasoning string // reasoning effort for reasoning models; "" omits the param
	http      *http.Client
	fallback  Brain
}

func NewLLMBrain(apiKey, model, baseURL, reasoning string) *LLMBrain {
	return &LLMBrain{
		apiKey:    apiKey,
		model:     model,
		baseURL:   strings.TrimRight(baseURL, "/"),
		reasoning: reasoning,
		http:      &http.Client{Timeout: 30 * time.Second},
		fallback:  &ScriptedBrain{},
	}
}

func (b *LLMBrain) Name() string { return "llm:" + b.model }

func (b *LLMBrain) Decide(ctx context.Context, w World) Action {
	act, err := b.ask(ctx, w)
	if err != nil {
		slog.Warn("llm brain failed, using fallback", "error", err)
		return b.fallback.Decide(ctx, w)
	}
	return act
}

// Observe forwards feedback to the fallback brain so its build blacklist stays
// useful whenever the LLM call fails and we fall back.
func (b *LLMBrain) Observe(action Action, err error) { b.fallback.Observe(action, err) }

// systemPrompt frames the model as a player agent that must reply with one
// JSON action. Kept terse — the action vocabulary mirrors ActionKind.
const systemPrompt = `You control a symbiont in the GEO-MMO "Ezra". You are a player agent on a real-world map.
Each turn you receive your state and nearby map cells. Reply with EXACTLY ONE JSON object, no prose:
{"action":"move|place|capture|attack|breach|idle","lat":<float>,"lng":<float>,"tower_id":"<id>","rift_id":"<id>","cell_id":"<id>","reason":"<short>"}
Rules:
- "move": walk toward lat/lng (you move gradually, anti-cheat caps speed).
- "place": build a beacon at your current location (needs materials; only on a free cell you stand on).
- "capture": lockpick an enemy tower you are within ~45m of; set tower_id.
- "attack": fight a rift; set rift_id (a squad of fighters is formed automatically). No proximity needed.
- "breach": (symbionts) puncture an enemy dome, opening a rift under it; set cell_id from a pvp_target.
- "idle": wait (almost never — there is usually something to do).
Be DECISIVE and aggressive. Priority each turn: if pvp_targets is non-empty → "breach" one now; else if any cell has a rift_id → "attack" it now; else if you have materials and stand on/near a free cell → "place"; only "move" toward a target when nothing is actionable yet. Do NOT just keep moving turn after turn — commit to breach/attack/place whenever possible.`

// llmWorldView is the compact JSON we feed the model (the full CellDTO is noisy).
type llmWorldView struct {
	Self struct {
		Lat       float64 `json:"lat"`
		Lng       float64 `json:"lng"`
		Level     int     `json:"level"`
		Energy    int     `json:"energy"`
		Materials int     `json:"materials"`
	} `json:"self"`
	Cells      []llmCell   `json:"cells"`
	PvpTargets []llmTarget `json:"pvp_targets,omitempty"`
}

type llmTarget struct {
	CellID   string  `json:"cell_id"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	Defender string  `json:"defender_name,omitempty"`
}

type llmCell struct {
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	DistanceM float64 `json:"distance_m"`
	Free      bool    `json:"free"`
	TowerID   string  `json:"tower_id,omitempty"`
	Owner     string  `json:"tower_owner,omitempty"`
	Enemy     bool    `json:"enemy_tower,omitempty"`
	RiftID    string  `json:"rift_id,omitempty"`
}

func (b *LLMBrain) buildView(w World) llmWorldView {
	var v llmWorldView
	v.Self.Lat = w.Self.Position.Lat
	v.Self.Lng = w.Self.Position.Lng
	v.Self.Level = w.Self.Level
	v.Self.Energy = w.Self.Energy
	v.Self.Materials = w.Self.Materials

	// Cap the cell list so the prompt stays small; nearest-first is most useful.
	const maxCells = 25
	for i := range w.Cells {
		c := w.Cells[i]
		lc := llmCell{
			Lat:       c.Lat,
			Lng:       c.Lng,
			DistanceM: geo.Haversine(v.Self.Lat, v.Self.Lng, c.Lat, c.Lng),
			Free:      c.TowerID == nil,
		}
		if c.Tower != nil {
			lc.TowerID = c.Tower.ID
			lc.Owner = c.Tower.OwnerID
			lc.Enemy = c.Tower.OwnerID != w.Self.ID
		}
		if c.RiftID != nil {
			lc.RiftID = *c.RiftID
		}
		v.Cells = append(v.Cells, lc)
		if len(v.Cells) >= maxCells {
			break
		}
	}
	for i := range w.PvpTargets {
		t := w.PvpTargets[i]
		v.PvpTargets = append(v.PvpTargets, llmTarget{
			CellID: t.CellID, Lat: t.Lat, Lng: t.Lng, Defender: t.DefenderName,
		})
	}
	return v
}

type orRequest struct {
	Model          string         `json:"model"`
	Messages       []orMessage    `json:"messages"`
	ResponseFormat map[string]any `json:"response_format,omitempty"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	Reasoning      map[string]any `json:"reasoning,omitempty"`
}

type orMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type orResponse struct {
	Choices []struct {
		Message orMessage `json:"message"`
	} `json:"choices"`
}

func (b *LLMBrain) ask(ctx context.Context, w World) (Action, error) {
	view, err := json.Marshal(b.buildView(w))
	if err != nil {
		return Action{}, err
	}

	orReq := orRequest{
		Model: b.model,
		Messages: []orMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: string(view)},
		},
		// Force valid JSON output — small models (e.g. gpt-oss-20b) otherwise
		// emit prose or truncated objects, which forced a scripted fallback.
		ResponseFormat: map[string]any{"type": "json_object"},
		MaxTokens:      2500,
	}
	// Reasoning effort only for reasoning models (gpt-oss): low effort stops it
	// from burning the token budget thinking. Non-reasoning models (qwen, llama)
	// reject the param with a 400, so it's omitted unless explicitly configured.
	if b.reasoning != "" {
		orReq.Reasoning = map[string]any{"effort": b.reasoning}
	}
	reqBody, _ := json.Marshal(orReq)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		b.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return Action{}, err
	}
	if b.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+b.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.http.Do(req)
	if err != nil {
		return Action{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return Action{}, fmt.Errorf("openrouter http %d: %s", resp.StatusCode, raw)
	}

	var or orResponse
	if err := json.Unmarshal(raw, &or); err != nil {
		return Action{}, err
	}
	if len(or.Choices) == 0 {
		return Action{}, fmt.Errorf("openrouter returned no choices")
	}
	return parseAction(or.Choices[0].Message.Content)
}

// parseAction extracts the JSON action object from the model's reply, tolerating
// surrounding prose or markdown fences.
func parseAction(s string) (Action, error) {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return Action{}, fmt.Errorf("no JSON object in reply: %q", s)
	}
	var raw struct {
		Action  string  `json:"action"`
		Lat     float64 `json:"lat"`
		Lng     float64 `json:"lng"`
		TowerID string  `json:"tower_id"`
		RiftID  string  `json:"rift_id"`
		CellID  string  `json:"cell_id"`
		Reason  string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(s[start:end+1]), &raw); err != nil {
		return Action{}, err
	}

	a := Action{Lat: raw.Lat, Lng: raw.Lng, TowerID: raw.TowerID, RiftID: raw.RiftID, CellID: raw.CellID, Reason: raw.Reason}
	switch strings.ToLower(raw.Action) {
	case "move":
		a.Kind = ActMoveTo
	case "place":
		a.Kind = ActPlace
	case "capture":
		a.Kind = ActCapture
	case "attack":
		a.Kind = ActAttackRift
	case "breach":
		a.Kind = ActBreach
	default:
		a.Kind = ActIdle
	}
	return a, nil
}
