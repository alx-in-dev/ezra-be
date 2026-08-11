> **Historical planning document.** Written early, before the Go backend was
> built out. It describes an earlier design in TypeScript/BullMQ pseudocode,
> mentions a Unity client and AWS (EC2/RDS/S3), and lists far fewer game
> systems than actually exist today. None of that matches the current
> implementation — the real stack is Go + chi + asynq + Postgres/PostGIS +
> Redis, deployed as Docker containers on a single VPS.
>
> Kept for historical context only. For the current design, use
> [`../ARCHITECTURE.md`](../ARCHITECTURE.md), [`../API.md`](../API.md), and
> [`../DOMAIN_GLOSSARY.md`](../DOMAIN_GLOSSARY.md) instead.

---

# 09. Технические заметки — Tech Notes

## 9.1 Стек (итог)

| Слой | Технология |
|---|---|
| Клиент | Unity 2022+ |
| Карта | Mapbox SDK для Unity |
| Backend | Go |
| БД основная | PostgreSQL + PostGIS |
| Кэш | Redis |
| Реалтайм (v1.1) | WebSocket |
| Авторизация | Firebase Authentication |
| Аналитика | Firebase Analytics |
| Push | Firebase Cloud Messaging (FCM) |
| Хранилище ассетов | AWS S3 + CloudFront |
| Инфраструктура | AWS EC2 + RDS |
| Очереди | asynq / worker queue |

---

## 9.2 Модели данных

### Player

```typescript
interface Player {
  id: string;                  // UUID
  firebase_uid: string;        // Firebase Auth UID
  username: string;
  level: number;               // 1–30
  xp: number;
  energy: number;              // текущий запас
  materials: number;
  army_limit: number;          // базово 50
  skill_points: SkillPoints;
  last_active: Date;
  position: {                  // последняя известная позиция
    lat: number;
    lng: number;
    updated_at: Date;
  };
  created_at: Date;
}

interface SkillPoints {
  defender: number;    // очки в ветке Защитника
  commander: number;   // очки в ветке Командира
  energist: number;    // очки в ветке Энергетика
}
```

### Cell (клетка карты)

```typescript
interface Cell {
  id: string;                  // "{lat}_{lng}" центр клетки
  lat: number;
  lng: number;
  infection: number;           // 0–100
  terrain: 'road' | 'building' | 'open';
  tower_id: string | null;     // FK → Tower
  rift_id: string | null;      // FK → Rift
  last_calculated: Date;       // когда сервер последний раз считал
}
```

### Tower (маяк)

```typescript
interface Tower {
  id: string;                  // UUID
  cell_id: string;             // FK → Cell
  owner_id: string;            // FK → Player
  level: number;               // 1–5
  type: 'standard' | 'amplified' | 'relay';
  hp: number;                  // текущие HP
  hp_max: number;              // макс HP (зависит от уровня)
  radius_m: number;            // радиус действия в метрах
  effect_per_hour: number;     // снижение заражения / час
  installed_at: Date;
  upgraded_at: Date;
}
```

### Unit (юнит армии)

```typescript
interface Unit {
  id: string;
  player_id: string;           // FK → Player
  type: 'fighter' | 'engineer' | 'scout' | 'medic';
  level: number;               // 1–5
  xp: number;
  hp: number;
  atk: number;
  squad_id: string | null;     // FK → Squad
  status: 'idle' | 'on_mission' | 'lost';
}
```

### Rift (разлом)

```typescript
interface Rift {
  id: string;
  cell_id: string;             // FK → Cell (центр)
  type: 'minor' | 'medium' | 'critical';
  intensity: number;           // 1–10
  radius_cells: number;        // радиус в клетках
  opened_at: Date;
  closed_at: Date | null;
  spirits_count: number;
}
```

### Pet (питомец)

```typescript
interface Pet {
  id: string;
  player_id: string;
  name: string;
  type: 'scout' | 'guardian' | 'energy';
  level: number;
  xp: number;
  status: 'idle' | 'on_task' | 'evolved';
  task: {
    type: 'search' | 'patrol';
    started_at: Date;
    returns_at: Date;
    loot_roll: number;         // 0–1, определяет качество лута
  } | null;
}
```

---

## 9.3 API — основные эндпоинты

### Авторизация

```
POST /auth/login
Body: { firebase_token: string }
Response: { player: Player, session_token: string }

POST /auth/register
Body: { firebase_token: string, username: string }
Response: { player: Player, session_token: string }
```

### Карта

```
GET /map/cells
Query: { lat, lng, radius_km }  // радиус загрузки
Response: { cells: Cell[] }

// Пример ответа:
{
  "cells": [
    {
      "id": "53.238_50.182",
      "lat": 53.238,
      "lng": 50.182,
      "infection": 64,
      "terrain": "building",
      "tower": { "level": 2, "owner": "player123", "type": "standard" },
      "rift": null
    }
  ]
}
```

### Маяки

```
POST /towers
Body: { cell_id, tower_type }
Response: { tower: Tower, energy_spent: number }

PATCH /towers/:id/upgrade
Body: { }  // проверяет условия на сервере
Response: { tower: Tower, upgrade_started_at: Date, ready_at: Date }

DELETE /towers/:id
Response: { resources_returned: { energy, materials } }
```

### Армия

```
GET /units
Response: { units: Unit[], army_count: number, army_limit: number }

POST /squads
Body: { name: string, unit_ids: string[] }
Response: { squad: Squad }

POST /squads/:id/send
Body: { mission_type: 'attack_rift' | 'patrol', target_id: string }
Response: { mission: Mission, returns_at: Date }
```

### Бой

```
POST /battles/start
Body: { target_type: 'rift' | 'network_event', target_id, squad_id }
Response: { battle_id: string, enemy: EnemyGroup, your_squad: Squad }

POST /battles/:id/action
Body: { action: 'attack' | 'defend' | 'energy' | 'retreat' }
Response: { round_result: RoundResult, battle_status: 'ongoing' | 'won' | 'lost' }
```

### Питомец

```
POST /pets/:id/send
Body: { task_type: 'search' | 'patrol', duration_minutes: number }
Response: { pet: Pet, returns_at: Date }

POST /pets/:id/recall
Response: { pet: Pet, loot: Loot }
```

---

## 9.4 Серверная логика заражения

### Задача (BullMQ Job)

```typescript
// Выполняется каждые 5 минут
async function recalculateInfection(cell: Cell): Promise<void> {
  const towers = await getTowersInRadius(cell, 400); // все маяки в радиусе
  const rifts = await getRiftsNearby(cell, 200);     // разломы в 200 м
  const neighbors = await getNeighborCells(cell);

  const growth = calculateGrowth(rifts, neighbors);
  const effect = calculateTowerEffect(towers, cell);
  const cityLink = await getCityLinkBonus(cell);

  const newInfection = Math.min(100, Math.max(0,
    cell.infection + growth - effect - cityLink
  ));

  await updateCell(cell.id, { infection: newInfection });

  // Проверить: нужно ли открыть разлом?
  if (newInfection > 75 && !cell.rift_id) {
    await maybeSpawnRift(cell, newInfection);
  }
}
```

---

## 9.5 Клиентская архитектура (Unity)

```
Assets/
├── Scripts/
│   ├── Core/
│   │   ├── GameManager.cs
│   │   ├── NetworkManager.cs  (REST + polling)
│   │   └── EventBus.cs
│   ├── Map/
│   │   ├── MapController.cs   (Mapbox)
│   │   ├── CellRenderer.cs    (окраска заражения)
│   │   ├── TowerPlacer.cs
│   │   └── RiftMarker.cs
│   ├── GPS/
│   │   ├── LocationService.cs
│   │   └── AntiCheat.cs       (проверка скорости)
│   ├── UI/
│   │   ├── MapHUD.cs
│   │   ├── ArmyPanel.cs
│   │   ├── BattleScreen.cs
│   │   └── PetPanel.cs
│   └── Data/
│       ├── PlayerData.cs
│       ├── CellData.cs
│       └── TowerData.cs
├── Prefabs/
│   ├── Tower/
│   ├── Rift/
│   └── Effects/
└── ScriptableObjects/
    ├── TowerConfig.asset
    ├── UnitConfig.asset
    └── GameBalance.asset   ← все числа баланса здесь
```

---

## 9.6 Античит (базовый)

```typescript
// Серверная проверка перед действием
async function validatePlayerPosition(
  playerId: string,
  claimedLat: number,
  claimedLng: number,
  targetCellId: string
): Promise<ValidationResult> {

  const lastPosition = await getLastPosition(playerId);
  const timeDelta = Date.now() - lastPosition.updated_at;
  const distance = geoDistance(lastPosition, { lat: claimedLat, lng: claimedLng });

  // Скорость в км/ч
  const speed = (distance / 1000) / (timeDelta / 3600000);

  if (speed > 50) {
    return { valid: false, reason: 'impossible_speed' };
  }

  const distanceToTarget = geoDistance(
    { lat: claimedLat, lng: claimedLng },
    getCellCenter(targetCellId)
  );

  if (distanceToTarget > 30) {
    return { valid: false, reason: 'too_far_from_target' };
  }

  return { valid: true };
}
```

---

## 9.7 Нагрузка (расчёт для MVP)

При 1000 активных игроков в регионе:

| Операция | Частота | RPS |
|---|---|---|
| Обновление позиции | каждые 10 сек | 100 |
| Загрузка клеток | раз в 30 сек | 33 |
| Действия (маяк, бой) | ~1 / мин на игрока | 17 |
| Пересчёт заражения (cron) | каждые 5 мин, ~10k клеток | фоновый |

**Вывод:** для MVP достаточно 1 EC2 t3.medium + RDS db.t3.small + Redis t3.micro.
