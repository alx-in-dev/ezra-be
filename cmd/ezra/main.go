package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"

	"github.com/ezra-game/server/internal/achievement"
	"github.com/ezra-game/server/internal/auth"
	"github.com/ezra-game/server/internal/battle"
	"github.com/ezra-game/server/internal/bestiary"
	"github.com/ezra-game/server/internal/capture"
	"github.com/ezra-game/server/internal/cell"
	"github.com/ezra-game/server/internal/citylink"
	"github.com/ezra-game/server/internal/faction"
	"github.com/ezra-game/server/internal/factionwar"
	"github.com/ezra-game/server/internal/hearth"
	"github.com/ezra-game/server/internal/hive"
	"github.com/ezra-game/server/internal/infection"
	"github.com/ezra-game/server/internal/item"
	"github.com/ezra-game/server/internal/legacy"
	"github.com/ezra-game/server/internal/nest"
	"github.com/ezra-game/server/internal/network"
	"github.com/ezra-game/server/internal/pet"
	"github.com/ezra-game/server/internal/platform"
	"github.com/ezra-game/server/internal/player"
	"github.com/ezra-game/server/internal/push"
	"github.com/ezra-game/server/internal/pvp"
	"github.com/ezra-game/server/internal/quest"
	"github.com/ezra-game/server/internal/realtime"
	"github.com/ezra-game/server/internal/resonance"
	"github.com/ezra-game/server/internal/rift"
	"github.com/ezra-game/server/internal/roster"
	"github.com/ezra-game/server/internal/shop"
	"github.com/ezra-game/server/internal/spire"
	"github.com/ezra-game/server/internal/squad"
	"github.com/ezra-game/server/internal/station"
	"github.com/ezra-game/server/internal/survivor"
	"github.com/ezra-game/server/internal/symbiont"
	"github.com/ezra-game/server/internal/tower"
	"github.com/ezra-game/server/internal/unit"
	"github.com/ezra-game/server/pkg/httputil"
	mw "github.com/ezra-game/server/pkg/middleware"
)

func main() {
	cfg := platform.LoadConfig()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// PostgreSQL
	db, err := platform.NewPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("postgres init failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Redis
	rdb, err := platform.NewRedis(ctx, cfg.RedisURL)
	if err != nil {
		slog.Error("redis init failed", "error", err)
		os.Exit(1)
	}
	defer rdb.Close()

	firebaseAuth, firebaseMsg, err := platform.NewFirebase(ctx, cfg.FirebaseCreds)
	if err != nil {
		slog.Error("firebase init failed", "error", err)
		os.Exit(1)
	}

	// Player
	playerRepo := player.NewPgRepository(db)
	playerSvc := player.NewService(playerRepo)
	playerHandler := player.NewHandler(playerSvc)

	// Auth
	authSvc := auth.NewService(rdb, firebaseAuth)
	authHandler := auth.NewHandler(authSvc, playerSvc)

	// Resources
	resourceSvc := player.NewResourceService(playerRepo)

	// Cells & Map
	cellRepo := cell.NewPgRepository(db)
	// Towers (repo created early so cell handler can use it)
	towerRepo := tower.NewPgRepository(db)
	// Lazy region seeder: on first /map/cells call in a new 0.1° grid cell,
	// queries Overpass for OSM features and populates cells + flags.
	overpassCli := platform.NewOverpassClient(cfg.OverpassEndpoints())
	cellSeeder := cell.NewSeeder(db, overpassCli)
	cellHandler := cell.NewHandler(cellRepo, &towerReaderAdapter{repo: towerRepo}, cellSeeder)

	// Quests & Streaks
	questRepo := quest.NewPgQuestRepository(db)
	questSvc := quest.NewQuestService(questRepo, resourceSvc)
	streakRepo := quest.NewPgStreakRepository(db)
	streakSvc := quest.NewStreakService(streakRepo, resourceSvc)
	streakSvc.SetCrystalsGranter(playerRepo) // 7/14/30-day milestones pay 💎, not just ⚡
	questHandler := quest.NewHandler(questSvc, streakSvc, playerRepo)

	// Push notifications
	pushRepo := push.NewPgRepository(db)
	// Explicit nil check: assigning a nil *FirebaseMessaging straight into
	// the interface would make it non-nil inside the service (typed nil).
	var messenger push.Messenger
	if firebaseMsg != nil {
		messenger = firebaseMsg
	}
	pushSvc := push.NewService(pushRepo, rdb, playerRepo, messenger)
	pushWorker := push.NewWorker(pushSvc)
	pushHandler := push.NewHandler(pushSvc)

	// Realtime (B6): SSE hub. Tower notifications fan out to both Firebase push
	// and the live SSE channel so the defense race works without polling.
	realtimeHub := realtime.NewHub()
	realtimeHandler := realtime.NewHandler(realtimeHub)
	towerNotifier := &realtime.PushFanout{Inner: pushSvc, Hub: realtimeHub}

	// Tower service
	towerSvc := tower.NewService(towerRepo, cellRepo, playerRepo, resourceSvc, questSvc, towerNotifier)
	towerSvc.WithEvents(realtimeHub) // R4-6: live "beacon destroyed" to the owner
	towerHandler := tower.NewHandler(towerSvc, playerSvc)
	towerWorker := tower.NewWorker(towerRepo, resourceSvc)

	// Beacon network / dome (Core → links → dome). See docs/feature/beacon_network_dome.md.
	networkRepo := network.NewPgRepository(db)
	networkSvc := network.NewService(networkRepo, cellRepo, playerRepo, resourceSvc, cellSeeder)
	networkHandler := network.NewHandler(networkSvc)
	towerSvc.WithNetwork(networkSvc)      // auto-link on place, cascade on remove
	towerWorker.WithDomeArea(networkRepo) // dome area passive income (B3)

	// Legacy human network (R4-4a): ex-human beacons degrade stable→fading→offline
	// unless an active human nearby sustains them; the worker keeps stamps + domes
	// in sync.
	legacyWorker := legacy.NewWorker(legacy.NewRepository(db), networkSvc)

	// City Link (R4-4b): mutual-support alliances between players.
	cityLinkSvc := citylink.NewService(citylink.NewRepository(db)).WithEvents(realtimeHub)
	cityLinkHandler := citylink.NewHandler(cityLinkSvc)
	towerSvc.WithAllies(cityLinkSvc) // ally repair + ally destroyed-alert fan-out

	// Power plants (energy pillar, T-777): raise local beacon capacity off-grid.
	stationRepo := station.NewPgRepository(db)
	stationSvc := station.NewService(stationRepo, resourceSvc)
	stationHandler := station.NewHandler(stationSvc)
	stationWorker := station.NewWorker(stationSvc)
	towerSvc.WithStations(stationRepo) // station bonus feeds beacon-area capacity

	// Ephemeral Hearth (T-805): Symbiont muster point for a coordinated raid.
	hearthRepo := hearth.NewPgRepository(db)
	hearthSvc := hearth.NewService(hearthRepo)
	hearthHandler := hearth.NewHandler(hearthSvc)

	// Rifts
	riftRepo := rift.NewPgRepository(db)
	riftSvc := rift.NewService(riftRepo, cellRepo, pushSvc)
	riftWorker := rift.NewWorker(riftSvc)

	// Hives (E1 — Symbiont anti-network root)
	hiveRepo := hive.NewPgRepository(db)
	hiveSvc := hive.NewService(hiveRepo, riftSvc)
	hiveHandler := hive.NewHandler(hiveSvc)
	hiveWorker := hive.NewWorker(hiveSvc)

	// Nest (N3): the Symbiont home. playerRepo satisfies both crystalSpender
	// (relocate/rebuild) and ResonanceGranter (collect buffer→profile).
	nestRepo := nest.NewPgRepository(db)
	nestSvc := nest.NewService(nestRepo, cellRepo, playerRepo, playerRepo).
		WithEvents(realtimeHub) // R4-6: live "nest under attack" + ETA to the owner
	nestSvc.AddDefenseModifier(nest.NewSkillDefenderModifier(&nestDefenderReader{playerRepo})) // T-844: Defender skill hardens the nest
	nestHandler := nest.NewHandler(nestSvc)
	nestWorker := nest.NewWorker(nestSvc)

	// Units
	unitRepo := unit.NewPgRepository(db)
	unitSvc := unit.NewService(unitRepo, playerRepo)
	unitHandler := unit.NewHandler(unitSvc)
	unitWorker := unit.NewWorker(db, unitRepo)

	// Squads
	squadRepo := squad.NewPgRepository(db)
	squadSvc := squad.NewService(squadRepo, unitRepo)
	squadSvc.SetMissionDeps(resourceSvc, cellRepo, pushSvc) // D: timed patrol/capture rewards on return
	squadSvc.SetMissionGuards(playerRepo, resourceSvc)      // anti remote-farm: field range + dispatch cost
	squadHandler := squad.NewHandler(squadSvc)

	// Inventory / items (D5 epic loot + resonance signatures)
	itemRepo := item.NewPgRepository(db)
	itemSvc := item.NewService(itemRepo)
	itemHandler := item.NewHandler(itemSvc)

	// Achievements (retention, #9): live-derived milestones; crystal reward
	// granted once per unlock via playerRepo.IncrementCrystals.
	achievementSvc := achievement.NewService(achievement.NewPgRepository(db), playerRepo)
	achievementHandler := achievement.NewHandler(achievementSvc)

	// Bestiary (retention, T-660): collect spirit/rift/tower/spectrum entries.
	bestiarySvc := bestiary.NewService(bestiary.NewPgRepository(db), playerRepo)
	bestiaryHandler := bestiary.NewHandler(bestiarySvc)

	// Resonance endgame (D1): collect the signature spectrum → counter-wave.
	resonanceRepo := resonance.NewPgRepository(db)
	resonanceSvc := resonance.NewService(itemSvc, playerRepo, resonanceRepo)
	resonanceHandler := resonance.NewHandler(resonanceSvc)

	// Faction war (E2 slice): regional Human-vs-Symbiont balance + season score.
	factionWarSvc := factionwar.NewService(factionwar.NewPgRepository(db))
	factionWarSvc.SetRewarders(playerRepo, itemSvc) // season rewards: crystals + title
	factionWarHandler := factionwar.NewHandler(factionWarSvc)
	factionWarWorker := factionwar.NewWorker(factionWarSvc)

	// Spire (civic megastructure): collective regional build → infection suppression.
	spireSvc := spire.NewService(spire.NewPgRepository(db), itemSvc, resourceSvc)
	spireHandler := spire.NewHandler(spireSvc)
	spireWorker := spire.NewWorker(spireSvc)

	// Faction onboarding (E3): Contact (hive collapse) → choose Human/Symbiont.
	factionRepo := faction.NewPgRepository(db)
	factionSvc := faction.NewService(factionRepo)
	factionSvc.SetResonanceReader(playerRepo) // #6: live Symbiont Resonance in /faction
	factionHandler := faction.NewHandler(factionSvc)
	// E2: side-gate war points (a Symbiont closing rifts must not farm the
	// Human leaderboard) — wired late because factionRepo is created here.
	factionWarSvc.SetFactionReader(factionRepo)
	// T-800: Symbionts don't get the human toolkit (build/army/battle) — wired
	// late because factionSvc is created here, after these services.
	towerSvc.WithFaction(factionSvc)
	squadSvc.SetFactionChecker(factionSvc)
	stationSvc.WithFaction(factionSvc)
	hearthSvc.WithFaction(factionSvc) // T-805: Summon is Symbiont-only

	// #6 T-760/T-761: Symbiont under-dome status + soft-drain.
	symbiontSvc := symbiont.NewService(factionSvc, cellRepo, playerRepo, resourceSvc, playerRepo)
	symbiontSvc.WithCorroder(towerSvc)    // R4-2: "Перегрузить маяк" corrodes hostile beacons
	symbiontSvc.WithStations(stationSvc)  // T-804: Overload falls back to sabotaging a power plant
	symbiontSvc.WithHearth(hearthSvc)     // T-805: coordinated-raid Overload damage bonus
	symbiontSvc.WithPressure(cellRepo)    // T-806: world-wide pierced-cell gauge
	symbiontSvc.WithXP(playerRepo)        // R4-2: verbs feed Resonance XP (drives Resonance Level)
	symbiontSvc.WithWar(factionWarSvc)    // E2: Overload scores Symbiont war points
	symbiontSvc.WithPositions(playerRepo) // anti-spoof: verbs/drain use the server-side position
	symbiontWorker := symbiont.NewWorker(symbiontSvc, playerRepo)
	symbiontHandler := symbiont.NewHandler(symbiontSvc)
	hiveSvc.SetEmpowerDeps(factionSvc, resourceSvc) // R4-2: Symbionts can empower hives
	hiveSvc.SetResonanceGranter(playerRepo)         // #6: empower feeds portable Resonance

	// PvP (v1.1): Symbiont Dome Breach against Human networks.
	pvpSvc := pvp.NewService(pvp.NewPgRepository(db), factionSvc, riftSvc, resourceSvc, pushSvc, playerRepo)
	pvpSvc.WithEvents(realtimeHub) // R4-6: live "dome breached" to the defender
	pvpSvc.WithWar(factionWarSvc)  // E2: Dome Breach scores Symbiont war points
	pvpHandler := pvp.NewHandler(pvpSvc)

	// Battles
	battleRepo := battle.NewPgRepository(db)
	battleSquadProvider := battle.NewPgSquadProvider(db)
	battleSvc := battle.NewService(battleRepo, riftSvc, battleSquadProvider, resourceSvc, playerSvc, playerRepo, questSvc)
	battleSvc.SetUnitProgressor(unitSvc)                      // D3: surviving units earn battle XP and level up
	battleSvc.SetItemGranter(itemSvc)                         // D5: critical-rift shard bosses drop resonance signatures
	battleSvc.SetHiveProvider(hiveSvc)                        // E1: squads can assault and collapse hives
	battleSvc.SetNestProvider(nestSvc)                        // N3: squads can siege a Symbiont home nest (never one-shot)
	battleSvc.SetFactionScorer(factionWarSvc)                 // E2: victories score Human faction-war points
	battleSvc.SetFactionContacter(factionSvc)                 // E3: hive collapse = Contact (unlocks side choice)
	battleSvc.SetDiscoverer(bestiarySvc)                      // T-660: engaging logs spirit/rift types to the bestiary
	battleSvc.SetRiftLimiter(battle.NewRedisRiftLimiter(rdb)) // anti-inflation: daily rift-closure cap by tier (docs/06 §6.3)
	battleSvc.SetFactionChecker(factionSvc)                   // T-800: Symbionts don't fight normal PvE battles
	battleHandler := battle.NewHandler(battleSvc)

	// Pets
	petRepo := pet.NewPgRepository(db)
	petSvc := pet.NewService(petRepo, towerRepo, cellRepo, playerRepo, resourceSvc, questSvc, pushSvc)
	petSvc.SetItemGranter(itemSvc) // D1: pet evolution drops a resonance signature
	// contact_step soft-lock guard: entering the Contact beat guarantees an
	// open rift near the player (areas without a hive may have none).
	petSvc.SetContactRiftEnsurer(func(ctx context.Context, lat, lng float64) error {
		_, err := riftSvc.EnsureContactRift(ctx, lat, lng)
		return err
	})
	petHandler := pet.NewHandler(petSvc, playerSvc)
	petWorker := pet.NewWorker(petSvc)

	// R4-2 roster L1: seed the Resonance Pool from human progression (units +
	// beacons + pets) when a player sides with the Symbionts; surface RL/XP/cap.
	rosterSvc := roster.NewService(unitRepo, towerRepo, petRepo, playerRepo)
	factionSvc.SetRosterInitializer(rosterSvc)
	factionSvc.SetProgressReader(playerRepo)
	factionSvc.SetOnboardingFinisher(playerSvc) // onboarding: final faction choice → completed
	// R4-2 roster L2: entity roster — materialize from the Pool, control cap by RL,
	// autonomous tick corrodes beacons / amplifies rifts.
	entityRepo := roster.NewPgEntityRepository(db)
	rosterSvc.WithEntities(entityRepo, playerRepo)
	rosterSvc.WithEffectors(towerSvc, riftSvc) // tick effectors (CorrodeTower / AmplifyRift)
	symbiontSvc.WithRoster(rosterSvc)          // command screen + assign/recall
	symbiontSvc.WithOnboarding(playerSvc)      // symbiont tutorial: relax verb gate + advance step on action
	rosterWorker := roster.NewWorker(rosterSvc)

	// Capture
	captureSvc := capture.NewService(towerRepo, playerRepo, cellRepo, pushSvc)
	captureHandler := capture.NewHandler(captureSvc)

	// Survivors
	survivorSvc := survivor.NewService(unitRepo, playerRepo).
		WithDomes(survivor.NewPgDomeReader(db)).
		WithFaction(factionSvc) // T-800: Symbionts don't recruit an army
	survivorHandler := survivor.NewHandler(survivorSvc)

	// Shop (T-540: register catalog/buy/crystals/subscription routes)
	shopRepo := shop.NewPgRepository(db)
	shopSvc := shop.NewService(shopRepo, playerRepo, unitRepo, petRepo)
	shopHandler := shop.NewHandler(shopSvc)
	shopWorker := shop.NewWorker(shopSvc)

	// Infection engine
	infectionSvc := infection.NewService(cellRepo, riftRepo, towerRepo).
		WithBatchRepository(infection.NewPgRepository(db))
	infectionWorker := infection.NewWorker(infectionSvc)

	// Asynq worker server
	asynqSrv := platform.NewAsynqServer(cfg.RedisURL)
	mux := asynq.NewServeMux()
	mux.HandleFunc(infection.TypeRecalculate, infectionWorker.HandleRecalculate)
	mux.HandleFunc(infection.TypeTideAdvance, infectionWorker.HandleTideAdvance)
	mux.HandleFunc(hive.TypePulse, hiveWorker.HandlePulse)
	mux.HandleFunc(nest.TypeTick, nestWorker.HandleTick)
	mux.HandleFunc(factionwar.TypeSettle, factionWarWorker.HandleSettle)
	mux.HandleFunc(spire.TypeLifecycle, spireWorker.HandleLifecycle)
	mux.HandleFunc(station.TypeLifecycle, stationWorker.HandleLifecycle)
	mux.HandleFunc(rift.TypeExpand, riftWorker.HandleExpand)
	mux.HandleFunc(rift.TypeSpawnOrganic, riftWorker.HandleSpawnOrganic)
	mux.HandleFunc(tower.TypeAccruePassiveIncome, towerWorker.HandleAccruePassiveIncome)
	mux.HandleFunc(tower.TypePressureTick, func(ctx context.Context, _ *asynq.Task) error {
		return towerSvc.ApplyPressure(ctx)
	})
	mux.HandleFunc(pet.TypeAutoClaim, petWorker.HandleAutoClaim)
	mux.HandleFunc(unit.TypeArmyDecay, unitWorker.HandleDecay)
	mux.HandleFunc(push.TypeSend, pushWorker.HandleSend)
	mux.HandleFunc(shop.TypeExpireSubscriptions, shopWorker.HandleExpireSubscriptions)
	mux.HandleFunc(roster.TypeEntityTick, rosterWorker.HandleEntityTick)
	mux.HandleFunc(symbiont.TypeDrainTick, symbiontWorker.HandleDrainTick)
	mux.HandleFunc(legacy.TypeDegrade, legacyWorker.HandleDegrade)
	mux.HandleFunc(squad.TypeCompleteMissions, func(ctx context.Context, _ *asynq.Task) error {
		return squadSvc.CompleteDueMissions(ctx)
	})

	go func() {
		slog.Info("asynq worker starting")
		if err := asynqSrv.Run(mux); err != nil {
			slog.Error("asynq worker error", "error", err)
		}
	}()

	// Asynq scheduler (infection recalculation every 5 minutes)
	asynqScheduler := platform.NewAsynqScheduler(cfg.RedisURL)
	// T-827: Unique dedups overlapping recalcs — two concurrent BatchRecalculate
	// ticks deadlocked (40P01) in the N0 stress. The lock releases when a tick
	// finishes; a still-running tick makes the next @5m enqueue skip (logged as
	// an expected "task already exists" enqueue error, not a failure).
	if _, err := asynqScheduler.Register("@every 5m", infection.NewRecalculateTask(), asynq.Unique(5*time.Minute)); err != nil {
		slog.Error("asynq scheduler register failed", "error", err)
		os.Exit(1)
	}
	// T-827: the @1h heavies fire on distinct minute offsets (non-multiples of 5,
	// so they miss the @5m recalc too) to avoid the co-fire that stacked heavy DB
	// writers and produced the N0 deadlock.
	if _, err := asynqScheduler.Register("7 * * * *", rift.NewExpandTask()); err != nil {
		slog.Error("asynq scheduler register rift:expand failed", "error", err)
		os.Exit(1)
	}
	// Organic rift spawn (canon "infection>75% breeds rifts"), density-capped.
	if _, err := asynqScheduler.Register("@every 10m", rift.NewSpawnOrganicTask()); err != nil {
		slog.Error("asynq scheduler register rift:spawn_organic failed", "error", err)
		os.Exit(1)
	}
	// D2: the antagonist tide channels infection toward player beacons every 30m.
	if _, err := asynqScheduler.Register("@every 30m", infection.NewTideAdvanceTask()); err != nil {
		slog.Error("asynq scheduler register infection:tide failed", "error", err)
		os.Exit(1)
	}
	// E1: the Symbiont hive lifecycle (pulse infection, seed rifts, grow) hourly.
	if _, err := asynqScheduler.Register("23 * * * *", hive.NewPulseTask()); err != nil {
		slog.Error("asynq scheduler register hive:pulse failed", "error", err)
		os.Exit(1)
	}
	// N3: nest lifecycle (trickle + support decay). Every 5m at a :04 offset so
	// it never fires in lock-step with infection:recalculate (@every 5m at :00)
	// — jitter guards against the 40P01 deadlock from overlapping cell-touching
	// workers (N0 gotcha). Trickle/decay touch only `nests` today, but the offset
	// is future-proofing for the pocket-refresh pass (T-843).
	if _, err := asynqScheduler.Register("4-59/5 * * * *", nest.NewTickTask()); err != nil {
		slog.Error("asynq scheduler register nest:tick failed", "error", err)
		os.Exit(1)
	}
	// E2: settle finished faction-war seasons daily (rewards + reset is implicit).
	if _, err := asynqScheduler.Register("@every 24h", factionwar.NewSettleTask()); err != nil {
		slog.Error("asynq scheduler register factionwar:settle failed", "error", err)
		os.Exit(1)
	}
	// Spire upkeep: degrade/ruin Spires that lose regional support (every 6h).
	if _, err := asynqScheduler.Register("@every 6h", spire.NewLifecycleTask()); err != nil {
		slog.Error("asynq scheduler register spire:lifecycle failed", "error", err)
		os.Exit(1)
	}
	// Power-plant upkeep: degrade/ruin stations past their upkeep window (T-778).
	if _, err := asynqScheduler.Register("@every 6h", station.NewLifecycleTask()); err != nil {
		slog.Error("asynq scheduler register station:lifecycle failed", "error", err)
		os.Exit(1)
	}
	if _, err := asynqScheduler.Register("37 * * * *", tower.NewAccruePassiveIncomeTask()); err != nil {
		slog.Error("asynq scheduler register tower:accrue_passive_income failed", "error", err)
		os.Exit(1)
	}
	if _, err := asynqScheduler.Register("49 * * * *", tower.NewPressureTask()); err != nil {
		slog.Error("asynq scheduler register tower:pressure_tick failed", "error", err)
		os.Exit(1)
	}
	if _, err := asynqScheduler.Register("@every 1m", pet.NewAutoClaimTask()); err != nil {
		slog.Error("asynq scheduler register pet:auto_claim failed", "error", err)
		os.Exit(1)
	}
	// D: complete timed squad missions (patrol/capture) when they return.
	if _, err := asynqScheduler.Register("@every 1m", squad.NewCompleteMissionsTask()); err != nil {
		slog.Error("asynq scheduler register squad:complete_missions failed", "error", err)
		os.Exit(1)
	}
	if _, err := asynqScheduler.Register("@every 6h", unit.NewDecayTask()); err != nil {
		slog.Error("asynq scheduler register army:decay failed", "error", err)
		os.Exit(1)
	}
	if _, err := asynqScheduler.Register("@every 24h", shop.NewExpireSubscriptionsTask()); err != nil {
		slog.Error("asynq scheduler register shop:expire_subscriptions failed", "error", err)
		os.Exit(1)
	}
	if _, err := asynqScheduler.Register("@every 10m", roster.NewEntityTickTask()); err != nil {
		slog.Error("asynq scheduler register symbiont:entity_tick failed", "error", err)
		os.Exit(1)
	}
	// #6 T-761: under-dome soft-drain on the server clock (was client-poll-only,
	// so a client that didn't poll never bled).
	if _, err := asynqScheduler.Register("@every 2m", symbiont.NewDrainTickTask()); err != nil {
		slog.Error("asynq scheduler register symbiont:drain_tick failed", "error", err)
		os.Exit(1)
	}
	if _, err := asynqScheduler.Register("53 * * * *", legacy.NewDegradeTask()); err != nil {
		slog.Error("asynq scheduler register legacy:degrade failed", "error", err)
		os.Exit(1)
	}
	go func() {
		slog.Info("asynq scheduler starting")
		if err := asynqScheduler.Run(); err != nil {
			slog.Error("asynq scheduler error", "error", err)
		}
	}()

	// Router
	r := chi.NewRouter()
	r.Use(mw.Recovery)
	r.Use(mw.Logging)
	r.Use(mw.CORS)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		// Auth routes (public)
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(auth.AuthMiddleware(authSvc))
			r.Post("/auth/logout", authHandler.Logout)
			r.Get("/player", playerHandler.GetPlayer)
			r.Get("/profile", playerHandler.GetProfile)
			r.Get("/onboarding", playerHandler.GetOnboarding)
			r.Post("/onboarding/advance", playerHandler.AdvanceOnboarding)
			r.Patch("/profile/username", playerHandler.UpdateUsername)
			r.Get("/skills", playerHandler.GetSkills)
			r.Post("/skills/{id}/unlock", playerHandler.UnlockSkill)
			r.Patch("/player/position", playerHandler.UpdatePosition)
			r.Get("/map/cells", cellHandler.GetCells)
			r.Get("/map/fields", networkHandler.GetRegionFields)
			r.Post("/stations", stationHandler.Build)
			r.Post("/stations/upkeep", stationHandler.Upkeep)
			r.Get("/stations", stationHandler.List)
			r.Get("/achievements", achievementHandler.Get)
			r.Get("/bestiary", bestiaryHandler.Get)
			r.Get("/symbiont/status", symbiontHandler.Status)
			r.Post("/symbiont/raise", symbiontHandler.Raise)
			r.Post("/symbiont/overload", symbiontHandler.Overload)
			r.Get("/symbiont/recon", symbiontHandler.Recon)
			r.Post("/symbiont/hearth", hearthHandler.Summon)
			r.Get("/symbiont/pressure", symbiontHandler.Pressure)
			r.Get("/symbiont/entities", symbiontHandler.Entities)
			r.Post("/symbiont/entities/assign", symbiontHandler.AssignEntity)
			r.Post("/symbiont/entities/recall", symbiontHandler.RecallEntity)
			r.Post("/core", networkHandler.PlaceCore)
			r.Post("/core/relocate", networkHandler.RelocateCore)
			r.Post("/core/upgrade", networkHandler.UpgradeCore)
			r.Get("/network", networkHandler.GetNetwork)
			// City Link (R4-4b): mutual-support alliances.
			r.Get("/city-links", cityLinkHandler.List)
			r.Get("/city-links/search", cityLinkHandler.Search)
			r.Post("/city-links/request", cityLinkHandler.Request)
			r.Get("/city-links/requests", cityLinkHandler.Inbox)
			r.Post("/city-links/requests/{id}/accept", cityLinkHandler.Accept)
			r.Post("/city-links/requests/{id}/reject", cityLinkHandler.Reject)
			r.Post("/city-links/{id}/remove", cityLinkHandler.Remove)
			r.Get("/events", realtimeHandler.Stream)         // B6: SSE realtime channel
			r.Post("/events/test", realtimeHandler.SelfTest) // B6 QA: publish a sample event to self
			r.Post("/towers", towerHandler.Create)
			r.Patch("/towers/{id}/upgrade", towerHandler.Upgrade)
			r.Post("/towers/{id}/repair", towerHandler.Repair)
			r.Delete("/towers/{id}", towerHandler.Delete)
			r.Get("/towers/mine", towerHandler.GetMine)
			r.Get("/units", unitHandler.List)
			r.Post("/units", unitHandler.Recruit)
			r.Get("/items", itemHandler.List)
			r.Get("/hives", hiveHandler.List)
			r.Post("/hives/{id}/empower", hiveHandler.Empower)
			// Nest (N3): the Symbiont home.
			r.Get("/nest", nestHandler.Get)
			r.Get("/nests/nearby", nestHandler.Nearby)
			r.Post("/nest", nestHandler.Create)
			r.Post("/nest/relocate", nestHandler.Relocate)
			r.Post("/nest/feed", nestHandler.Feed)
			r.Post("/nest/collect", nestHandler.Collect)
			r.Post("/nest/repair", nestHandler.Repair)
			r.Get("/faction-war", factionWarHandler.Status)
			r.Post("/faction-war/settle", factionWarHandler.Settle)
			r.Get("/faction", factionHandler.Status)
			r.Post("/faction/choose", factionHandler.Choose)
			r.Get("/pvp/targets", pvpHandler.Targets)
			r.Post("/pvp/breach", pvpHandler.Breach)
			r.Get("/spire", spireHandler.Status)
			r.Post("/spire/contribute-fragment", spireHandler.ContributeFragment)
			r.Post("/spire/contribute", spireHandler.ContributeResources)
			r.Get("/resonance", resonanceHandler.Status)
			r.Post("/resonance/activate", resonanceHandler.Activate)
			r.Get("/squads", squadHandler.List)
			r.Post("/squads", squadHandler.Create)
			r.Post("/squads/{id}/send", squadHandler.Send)
			r.Post("/squads/{id}/recall", squadHandler.Return)
			r.Patch("/squads/{id}", squadHandler.Update)
			r.Delete("/squads/{id}", squadHandler.Disband)
			r.Post("/battles/start", battleHandler.Start)
			r.Post("/battles/{id}/action", battleHandler.Action)
			r.Post("/battles/{id}/overcharge", battleHandler.Overcharge)
			r.Get("/pets", petHandler.GetPets)
			r.Post("/pets/starter-claim", petHandler.ClaimStarter)
			r.Post("/pets/{id}/send", petHandler.Send)
			r.Post("/pets/{id}/recall", petHandler.Recall)
			r.Get("/quests", questHandler.GetQuests)
			r.Post("/quests/{id}/claim", questHandler.ClaimQuest)
			r.Post("/streaks/checkin", questHandler.CheckIn)
			r.Post("/towers/{id}/capture/lockpick", captureHandler.Lockpick)
			r.Post("/towers/{id}/capture/force", captureHandler.ForceCapture)
			r.Get("/survivors", survivorHandler.Spawn)
			r.Post("/survivors/recruit", survivorHandler.Recruit)
			r.Post("/player/push-token", pushHandler.RegisterToken)

			// Shop (T-540)
			r.Get("/shop/catalog", shopHandler.GetCatalog)
			r.Post("/shop/buy", shopHandler.Buy)
			r.Post("/shop/crystals", shopHandler.AddCrystals)
			r.Get("/shop/subscription", shopHandler.GetSubscription)
			r.Post("/shop/subscription/activate", shopHandler.ActivateSubscription)
		})
	})

	// Server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Start
	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			cancel()
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	slog.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	asynqScheduler.Shutdown()
	asynqSrv.Shutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("server stopped")
}

// nestDefenderReader adapts the player repo to nest.DefenderReader (T-844):
// the owner's Defender skill hardens their nest's siege window.
type nestDefenderReader struct {
	repo *player.PgRepository
}

func (a *nestDefenderReader) DefenderPoints(ctx context.Context, playerID string) (int, error) {
	p, err := a.repo.GetByID(ctx, playerID)
	if err != nil {
		return 0, err
	}
	if p == nil {
		return 0, nil
	}
	return p.Skills.Defender, nil
}

// towerReaderAdapter adapts tower.Repository to cell.TowerReader,
// breaking the import cycle between cell and tower packages.
type towerReaderAdapter struct {
	repo tower.Repository
}

func (a *towerReaderAdapter) GetByIDs(ctx context.Context, ids []string) ([]cell.TowerData, error) {
	towers, err := a.repo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]cell.TowerData, len(towers))
	for i, t := range towers {
		result[i] = cell.TowerData{
			ID:            t.ID,
			OwnerID:       t.OwnerID,
			Lat:           t.Lat,
			Lng:           t.Lng,
			Level:         t.Level,
			RadiusM:       t.RadiusM,
			EffectPerHour: t.EffectPerHour,
			HP:            t.HP,
			HPMax:         t.HPMax,
		}
	}
	return result, nil
}

func (a *towerReaderAdapter) CountAllInRadius(ctx context.Context, lat, lng, radiusM float64) (int, error) {
	return a.repo.CountAllInRadius(ctx, lat, lng, radiusM)
}
