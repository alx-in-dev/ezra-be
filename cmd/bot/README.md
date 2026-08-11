# Debug bot swarm

Headless bots that play Ezra as **symbiont players** for debugging multiplayer.
They authenticate with login/password (no Firebase), move around the map under
the anti-cheat speed cap, and take real player actions via the public REST API —
the same contract the Unity client uses. Run them alongside your own real
accounts to exercise interactions (capture, PvP, faction war, crowd density).

## Run

**Interactive launcher (easiest)** — pick everything from a console menu
(server, region, brain/model, factions & counts), then it spawns the swarm:

```bash
go run ./cmd/bot tui
```

**Direct via env** (scriptable / CI):

```bash
# 3 scripted bots against the local docker server
BOT_BASE_URL=http://localhost:8081 BOT_CENTER_LAT=53.13 BOT_CENTER_LNG=50.15 go run ./cmd/bot

# 10 humans vs 10 symbionts — two processes
export BOT_BASE_URL=http://localhost:8081 BOT_CENTER_LAT=53.13 BOT_CENTER_LNG=50.15
BOT_FACTION=human    BOT_PREFIX=hbot BOT_COUNT=10 go run ./cmd/bot &
BOT_FACTION=symbiont BOT_PREFIX=sbot BOT_COUNT=10 go run ./cmd/bot &

# LLM-driven bots (OpenRouter)
BOT_BRAIN=llm OPENROUTER_API_KEY=sk-or-... go run ./cmd/bot
```

> The local docker game server listens on **:8081** (the env default is :8080).
> Spawn in a seeded region (e.g. `53.13, 50.15`) — empty areas hang on the
> first cell fetch while the server lazy-seeds them from Overpass.

Ctrl-C stops the whole swarm cleanly. The `tui` launcher runs both factions in
one process; the env form runs one faction per process.

## Knobs (env)

| Var | Default | Meaning |
|---|---|---|
| `BOT_BASE_URL` | `http://localhost:8080` | server base URL |
| `BOT_COUNT` | `3` | number of bots |
| `BOT_PREFIX` | `dbgbot` | login = `<prefix><n>` (e.g. `dbgbot1`) |
| `BOT_PASSWORD` | `botpass123` | shared password |
| `BOT_CENTER_LAT/LNG` | Moscow | spawn center |
| `BOT_SPAWN_RADIUS_M` | `300` | bots scatter on a circle of this radius |
| `BOT_TICK_MS` | `2500` | decide/move cadence per bot |
| `BOT_WALK_SPEED_MS` | `8` | metres/sec; keep under server `MAX_SPEED_KMH` (default 50) |
| `BOT_BRAIN` | `scripted` | `scripted` (state machine) or `llm` (OpenRouter) |
| `OPENROUTER_API_KEY` | – | required for `BOT_BRAIN=llm` on OpenRouter (omit for local servers) |
| `OPENROUTER_MODEL` | `openai/gpt-oss-20b` | model for the LLM brain |
| `LLM_BASE_URL` | `https://openrouter.ai/api/v1` | any OpenAI-compatible endpoint; e.g. local Ollama `http://192.168.0.100:11434/v1` |

### LLM brain notes

The brain forces `response_format: json_object`. Any failed/empty reply falls
back to the scripted brain, so a flaky model never freezes the swarm.

- `LLM_REASONING_EFFORT=low` — set this **only** for reasoning models (gpt-oss),
  which otherwise burn their token budget "thinking" and truncate the JSON
  (~33% → ~13% fallback). Non-reasoning models (qwen, llama, mistral) reject the
  param with a 400, so leave it unset for them.

**Local Ollama** (free, no token cost) — set `LLM_BASE_URL` to the OpenAI-compat
endpoint, no API key:
```bash
BOT_BRAIN=llm LLM_BASE_URL=http://192.168.0.100:11434/v1 \
OPENROUTER_MODEL=llama3:8b BOT_TICK_MS=3500 BOT_COUNT=3 go run ./cmd/bot
```
Model pick on a single 16 GB GPU: `llama3:8b` is fast (~0.9 s warm) and reliable
for a per-tick decision; `qwen2.5-coder:14b` gives cleaner JSON but is too slow
under concurrency (6 bots → request queue → 30 s timeouts). Keep the bot count
low and `BOT_TICK_MS` ≥ 3500 so the single GPU keeps up.

## Anti-cheat note

`PATCH /player/position` validates speed (`geo.ValidateSpeed`, default 50 km/h).
The agent moves **gradually** (`WALK_SPEED_MS` per tick) so positions stay legal.
If you want bots to teleport for a quick test, raise the server's `MAX_SPEED_KMH`
or lower `BOT_TICK_MS`.

## Structure

- `config.go`  — env config
- `client.go`  — REST client; reuses server types (`player.Player`, `cell.CellDTO`, `tower.PlaceRequest`) so the compiler catches API drift
- `agent.go`   — per-bot loop: observe → decide → act; owns stepped movement
- `brain.go`   — `Brain` interface + `ScriptedBrain` goal state machine
- `brain_llm.go` — `LLMBrain` over OpenRouter (same `Brain` contract)

## Extending

Add an action: extend `ActionKind` + the `switch` in `agent.step`, add a client
method, and teach the brain when to emit it. Both brains share the `Action`
vocabulary, so a new action works for scripted and LLM bots alike.

The bots exercise the **full player action surface**:

- **Combat/territory** (per-tick decision, `brain.go`/`agent.go`): move, place
  Core (first structure) + beacon towers, lockpick/force capture, attack rifts
  (squad auto-formed, retreats from losing fights), symbiont dome breach.
- **Breadth sweep** (`housekeeping.go`, every ~8 ticks, best-effort): faction
  choose, daily streak check-in, claim ready quests, unlock skills, recruit
  survivors, upgrade/repair towers, core upgrade, pets (claim+send), hive
  empower (symbiont), spire contribute, resonance activate, shop (crystals+buy).
  Each logs `hk ok` / `hk skip` so a run shows exactly what fired.

Actions gated by server prerequisites (spire phase, resonance set, starter-pet
lock, skill points) log `hk skip` with the real rejection code — useful signal,
not a bug. Set `BOT_FACTION=human|symbiont`.

## Battle notes

- Only `rift` battles are usable: `hive` needs the hives service, `tutorial`
  needs the onboarding step. Rift start has **no proximity check** — the bot
  fights any rift it can see in `GetCells` (`CellDTO.RiftID`).
- A rift must be **open** and linked to a cell. To spawn one for testing:
  ```sql
  WITH ins AS (
    INSERT INTO rifts (cell_id, type, intensity, radius_cells, spirits_count)
    VALUES ('<cell_id>', 'minor', 1, 1, 3) RETURNING id, cell_id)
  UPDATE cells SET rift_id = ins.id FROM ins WHERE cells.id = ins.cell_id;
  ```
  A win closes the rift (`closed_at` set, cell `rift_id` cleared).
