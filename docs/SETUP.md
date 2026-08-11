# Настройка локальной разработки

## Что нужно заранее

- Go 1.25+
- Docker + Docker Compose (для Postgres/Redis или всего стека целиком)
- CLI `golang-migrate`, если хочешь запускать миграции с хоста
  (`brew install golang-migrate` или см.
  [документацию migrate](https://github.com/golang-migrate/migrate)) — не
  нужен, если запускаешь приложение только в Docker: контейнер всё равно не
  применяет миграции автоматически, `make migrate-up` в любом случае
  запускается против пробрасываемого порта Postgres.

## Вариант А — всё в Docker

```bash
docker compose up -d
```

Поднимает три контейнера:

- `app` — собирается из локального `Dockerfile`, проброшен на порт хоста
  `8081` (контейнер слушает `8080`).
- `postgres` — `postgis/postgis:16-3.4`, проброшен на `5432`.
- `redis` — `redis:7-alpine`, проброшен на `6379`.

`app` ждёт, пока обе зависимости пройдут healthcheck, прежде чем стартовать.
Также он задаёт два dev-friendly override'а, о которых стоит знать:

- `MAX_SPEED_KMH=100000` — античит по скорости фактически отключён, так
  что GPS-симуляторы ("FakeGPS") могут телепортировать тестовую позицию, не
  словив `impossible_speed`.
- `DOME_SUPPRESSION_PER_HOUR=600` — купола очищают заражение за минуты
  вместо продакшн ~30%/час, чтобы не ждать, пока подавление сработает.

Затем прогони миграции на проброшенный порт Postgres:

```bash
export DATABASE_URL="postgres://ezra:ezra@localhost:5432/ezra?sslmode=disable"
make migrate-up
```

`docker-compose.yml` также монтирует `./firebase-credentials.json` внутрь
контейнера в режиме read-only — этот файл не нужен, если ты не тестируешь
именно Firebase-логин (см. [Авторизацию](#авторизация) ниже); без него
сервер логирует "firebase auth is not configured", а вход по
login/password при этом продолжает нормально работать.

## Вариант Б — Go на хосте, инфраструктура в Docker

```bash
docker compose up -d postgres redis
export DATABASE_URL="postgres://ezra:ezra@localhost:5432/ezra?sslmode=disable"
export REDIS_URL="redis://localhost:6379"
make migrate-up
make run   # go run ./cmd/ezra, слушает :8080 по умолчанию
```

## Переменные окружения

У всех есть рабочие значения по умолчанию, кроме отмеченных; задавай только
то, что действительно нужно.

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `DATABASE_URL` | `postgres://ezra:ezra@localhost:5432/ezra?sslmode=disable` | DSN для Postgres |
| `REDIS_URL` | `redis://localhost:6379` | Redis — сессии + брокер asynq |
| `PORT` | `8080` | Порт, на котором слушает HTTP |
| `FIREBASE_CREDENTIALS_FILE` | `firebase-credentials.json` | Файл service-account для Firebase Admin SDK (нужен только для Firebase-логина) |
| `MAPBOX_TOKEN` | `""` | Mapbox, использование на стороне сервера |
| `OVERPASS_ENDPOINTS` | встроенные значения по умолчанию | URL Overpass (OSM) API через запятую, для ленивого посева клеток/регионов |
| `MAX_SPEED_KMH` | `50.0` | Порог скорости для античита обновления позиции (км/ч) |
| `DOME_SUPPRESSION_PER_HOUR` | значение по умолчанию из canon (`30.0`) | Скорость подавления заражения под активным куполом |
| `EZRA_ALLOW_DEV_IAP` | не задана | Если `"1"`, `/shop/buy` принимает фейковый чек `"dev"` для локального теста IAP |

У `cmd/bot` (рой ботов для отладки, не игровой сервер) свои переменные
окружения — см. `cmd/bot/README.md`.

## Авторизация

Firebase-проект для локальной разработки не нужен. `POST /auth/register` с
телом `login`/`password` создаёт игрока с паролем:

```bash
curl -s -X POST http://localhost:8081/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"login":"alice","password":"hunter22"}'
# → { "player": {...}, "session_token": "..." }

curl -s http://localhost:8081/api/v1/player \
  -H 'Authorization: Bearer <session_token>'
```

Если тебе конкретно нужно прогнать Firebase-логин (например, тест
интеграции с клиентом) — заведи Firebase-проект, положи его
service-account JSON как `firebase-credentials.json` в корень репозитория
(уже в gitignore — никогда не коммить его) и укажи
`FIREBASE_CREDENTIALS_FILE`, если положил файл в другое место.

## Посев карты

Отдельного seed-скрипта нет — клетки создаются лениво. Первый запрос
`GET /map/cells?lat=...&lng=...&radius_km=...` для нового региона ~0.1°
триггерит `cell.Seeder`, который запрашивает Overpass и заполняет эту
область. Чтобы прогреть регион вручную:

```bash
curl 'http://localhost:8081/api/v1/map/cells?lat=53.13&lng=50.15&radius_km=1.5' \
  -H 'Authorization: Bearer <session_token>'
```

## Проверка мультиплеерных фич

`cmd/bot` — headless-рой ботов, играющий через тот же публичный REST API,
что и реальный клиент; полезен, чтобы генерировать трафик для теста
faction war, PvP, захвата или просто плотности игроков без нескольких
реальных устройств. Использование — в `cmd/bot/README.md`; это отдельная
цель `go run`, не часть продакшн-сервера.

## Тесты

```bash
make test
```
