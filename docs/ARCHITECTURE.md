# Архитектура

## Слои

Каждый игровой пакет в `internal/` устроен одинаково:

```
model.go        типы / DTO
repository.go   доступ к Postgres — интерфейс + реализация поверх Pg
service.go      бизнес-логика, обращается к repository (и другим сервисам)
handler.go      HTTP-слой: декодировать запрос → вызвать сервис → закодировать ответ
worker.go       (опционально) обработчики asynq-задач для фоновой работы
*_test.go       unit-тесты против самописных фейков интерфейсов выше
```

`internal/canon` — единственное исключение: чистые константы и функции-правила
(гейтинг по release-stage, тюнинг боя/башен/сети), общие для остальных
пакетов, без своего доступа к БД.

DI-фреймворка нет. Всё собирается вручную, по порядку, в `cmd/ezra/main.go`:

```go
db, err := platform.NewPostgres(ctx, cfg.DatabaseURL)
...
towerRepo := tower.NewPgRepository(db)
towerSvc  := tower.NewService(towerRepo, ...)
towerHandler := tower.NewHandler(towerSvc)
...
r.Post("/towers", towerHandler.Create)
```

Файл длинный (~585 строк), но линейный и легко читается — если нужно узнать,
от чего зависит пакет, читай его конструктор в `main.go`. Один заметный
паттерн: `cell` и `tower` логически импортировали бы друг друга, поэтому
`main.go` определяет небольшой `towerReaderAdapter`, чтобы разорвать цикл, а
не сливать пакеты в один.

## HTTP

Роутинг — [chi](https://github.com/go-chi/chi). Всё живёт под `/api/v1`,
разбито на две группы:

- **Публичные**: `GET /health`, `POST /auth/register`, `POST /auth/login`.
- **Защищённые**: всё остальное, обёрнуто в `r.Group(...)` с
  `auth.AuthMiddleware`. Полный список роутов — в [API.md](API.md).

Глобальный middleware (`pkg/middleware`), применяется в таком порядке: panic
`Recovery` → логирование запроса `Logging` → `CORS`. Rate limiting
(`middleware.RateLimit`, token bucket на Redis) доступен для использования
на отдельных роутах, но глобально не подключён.

Общие хелперы для запроса/ответа (`httputil.Decode`, `httputil.JSON`,
`httputil.Error`, типизированные конструкторы ошибок вроде `NewBadRequest`/
`NewUnauthorized`) и контекст с ID игрока в рамках запроса живут в
`pkg/httputil`.

## Авторизация

Два независимых способа входа, оба заканчиваются одной и той же сессией:

1. **Firebase**: клиент присылает Firebase ID token → `auth.Service.
   VerifyFirebaseToken` проверяет его через Firebase Admin SDK
   (`platform.NewFirebase`) → получает `firebase_uid` → игрок находится или
   создаётся по этому UID.
2. **Login/password**: `login` + `password` → сверяется через bcrypt с
   сохранённым хешем.

В обоих случаях успешный `/auth/login` или `/auth/register` создаёт
случайный токен сессии (`auth.Service.CreateSession`), сохраняет
`session:<token> → playerID` в Redis с TTL 7 дней и возвращает его как
`session_token`. Каждый защищённый роут требует
`Authorization: Bearer <token>`; `auth.AuthMiddleware` резолвит его через
Redis и отдаёт 401 на всё отсутствующее/невалидное/протухшее.
`POST /auth/logout` просто удаляет ключ из Redis.

Для локальной разработки Firebase-проект не нужен — путь login/password
полностью самодостаточен. См. [SETUP.md](SETUP.md).

## Античит

`internal/player` валидирует каждое обновление `PATCH /player/position`: по
последней известной позиции игрока и заявленной новой считается неявная
скорость, и обновление отклоняется как `impossible_speed`, если она выше
`MAX_SPEED_KMH` (по умолчанию 50 км/ч). Это нужно, чтобы ловить
GPS-спуфинг ("телепорты"). Локальные/dev-окружения поднимают этот порог
(см. `docker-compose.yml`), чтобы GPS-симуляторы могли свободно двигаться.

## Реалтайм

`internal/realtime` — это поток **Server-Sent Events**, не WebSocket:
`GET /events` апгрейдится в долгоживущее SSE-соединение поверх
внутрипроцессного pub/sub `Hub` на игрока (один Go-канал на активную
подписку). Сейчас через него пушатся алерты обороны (например,
`tower_under_attack`), чтобы владелец узнавал мгновенно, а не ждал
следующего поллинга. Heartbeat-комментарий каждые 20 секунд не даёт
прокси закрыть соединение; `POST /events/test` публикует тестовое событие
в твой собственный поток для ручного QA.

Hub работает в одном инстансе/в памяти — если сервер когда-нибудь будет
масштабироваться горизонтально, тут понадобится fan-out на Redis (или
аналоге).

## Фоновые задачи

Один бинарник, `cmd/ezra`, запускает три вещи как горутины в одном
процессе: HTTP-сервер, asynq **worker** (роутинг тип задачи → обработчик
через `asynq.ServeMux`) и asynq **scheduler** (регистрации в стиле cron,
`@every`). Redis — брокер задач. Отдельного бинарника воркера для деплоя
нет.

| Интервал | Задача | Пакет |
|---|---|---|
| 1м | `pet:auto_claim` | pet |
| 1м | `squad:complete_missions` | squad |
| 2м | `symbiont:drain_tick` | symbiont |
| 5м | `infection:recalculate` | infection |
| 10м | `rift:spawn_organic` | rift |
| 10м | `roster:entity_tick` | roster |
| 30м | `infection:tide_advance` | infection |
| 1ч | `rift:expand` | rift |
| 1ч | `hive:pulse` | hive |
| 1ч | `tower:accrue_passive_income` | tower |
| 1ч | `tower:pressure_tick` | tower |
| 1ч | `legacy:degrade` | legacy |
| 6ч | `spire:lifecycle` | spire |
| 6ч | `station:lifecycle` | station |
| 6ч | `unit:army_decay` | unit |
| 24ч | `factionwar:settle` | factionwar |
| 24ч | `shop:expire_subscriptions` | shop |

## Слой данных

- **PostgreSQL + PostGIS**: подключение через `pgx` (`internal/platform/
  postgres.go`); репозитории пишут SQL руками, без ORM. PostGIS включается
  в первой миграции; `cells.geom` — колонка `GEOMETRY(Point, 4326)` с
  `GIST`-индексом для пространственных запросов. `uuid-ossp` даёт
  `uuid_generate_v4()` для первичных ключей.
- **Redis**: сессии (см. Авторизацию выше) и брокер/scheduler asynq.
- **Миграции**: [golang-migrate](https://github.com/golang-migrate/migrate),
  `migrations/`, строгие пары `NNN_description.up.sql` / `.down.sql` (~96
  файлов на момент написания). Запускаются через `make migrate-up` /
  `make migrate-down`. ORM-based авто-миграций нет — каждое изменение схемы
  это новая нумерованная пара.

## Карта пакетов

| Пакет | Отвечает за | Воркер |
|---|---|---|
| `achievement` | Достижения на основе метрик, разблокировки | — |
| `auth` | Вход (Firebase + пароль), сессии в Redis | — |
| `battle` | Пошаговый бой против разломов/ульев | — |
| `bestiary` | Каталог открытых врагов/башен/спектров | — |
| `canon` | Центральные игровые константы и чистые функции | — |
| `capture` | Захват башни: lockpick / force | — |
| `cell` | Клетки карты, ленивый посев мира через Overpass | — |
| `citylink` | Альянсы между игроками | — |
| `faction` | Контакт/выбор/статус фракции | — |
| `factionwar` | Сезонный счёт фракций | ежедневный settle |
| `hive` | Жизненный цикл улья симбионтов | ежечасный pulse |
| `infection` | Пересчёт заражения клеток, механика "прилива" | 5м recalc, 30м tide |
| `item` | Инвентарь | — |
| `legacy` | Деградация "легаси"-башен (бывших) | ежечасный degrade |
| `network` | Установка/перенос/апгрейд Core, региональные поля | — |
| `pet` | Заявка/отправка/возврат питомца | 1м auto-claim |
| `platform` | Инфраструктура: Postgres, Redis, Firebase, Mapbox, Overpass, asynq, конфиг баланса | — |
| `player` | Профиль, ресурсы, онбординг, позиция/античит, навыки | — |
| `push` | Push-уведомления через FCM | очередь отправки |
| `pvp` | Список PvP-целей + пробитие купола | — |
| `quest` | Дневные квесты, стрик за вход | — |
| `realtime` | SSE-поток событий | — |
| `resonance` | Статус/активация Resonance Level симбионтов | — |
| `rift` | Жизненный цикл разломов | ежечасный expand, 10м organic spawn |
| `roster` | Ростер сущностей симбионта | 10м entity tick |
| `shop` | IAP-каталог, покупки, подписки | 24ч expire |
| `spire` | Эндгейм-структура Spire | 6ч lifecycle |
| `squad` | Создание/отправка/возврат отряда, миссии | 1м complete missions |
| `station` | Электростанции, построенные игроками | 6ч lifecycle |
| `survivor` | Вербуемые NPC | — |
| `symbiont` | Статус/подъём/перегрузка/разведка симбионта | 2м drain tick |
| `tower` | Постройка/ремонт/удаление башни, доход, давление | ежечасный доход, ежечасное давление |
| `unit` | Юниты армии | 6ч decay |

`pkg/` содержит инфраструктурно-нейтральный общий код: `pkg/httputil`
(HTTP-хелперы), `pkg/middleware` (CORS/логирование/rate-limit/recovery),
`pkg/geo` (математика расстояний).

## Тесты

Бизнес-логика покрыта unit-тестами с самописными in-memory фейками для
repository/collaborator-интерфейсов каждого пакета — без mock-фреймворков и
без docker-compose-based интеграционного набора. Пример паттерна — любой
`internal/*/service_test.go` (например, `internal/achievement/service_test.go`).
Запуск всего: `make test`.

## Гейтинг по релизам

Бэкенд в любой момент времени закреплён за одним `canon.ReleaseStage`
(`mvp` / `v1.0` / `v1.1`); фичи могут объявлять минимальную стадию и
централизованно гейтиться через `canon.IsFeatureAvailable`. Что реально
доступно прямо сейчас — смотри в `internal/canon/canon.go`, не полагайся
на предположения.
