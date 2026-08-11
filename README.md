# Ezra — Backend

Бэкенд **Ezra** — гео-MMO (карта реального мира, GPS-строительство базы,
заражение территории, PvP/PvE). Этот репозиторий — только Go-сервер, клиент
игры живёт в отдельном репозитории.

## Стек

| Слой | Технология |
|---|---|
| Язык | Go 1.25 |
| HTTP-роутер | [chi](https://github.com/go-chi/chi) |
| База данных | PostgreSQL + PostGIS |
| Кэш / сессии / очередь задач | Redis |
| Фоновые задачи | [asynq](https://github.com/hibiken/asynq) (worker + scheduler в том же процессе) |
| Авторизация | Firebase ID token **или** login/password (bcrypt) |
| Push-уведомления | Firebase Cloud Messaging |
| Реалтайм | Server-Sent Events (`GET /events`) |
| Миграции | [golang-migrate](https://github.com/golang-migrate/migrate) |

Один бинарник (`cmd/ezra`) обслуживает HTTP, запускает asynq worker и asynq
scheduler — всё как горутины в одном процессе. Отдельного бинарника для
воркера нет.

## Быстрый старт (локальная разработка)

Нужны Docker и Go 1.25+.

```bash
# 1. Поднять Postgres + Redis + приложение в контейнерах
cp .env.example .env   # нужно, только если запускаешь приложение вне Docker
docker compose up -d

# 2. Прогнать миграции (с хоста, на контейнерный Postgres)
export DATABASE_URL="postgres://ezra:ezra@localhost:5432/ezra?sslmode=disable"
make migrate-up

# 3. Проверить, что всё живо
curl http://localhost:8081/health
```

`docker-compose.yml` пробрасывает приложение на порт хоста `8081` и ослабляет
таймеры античита/заражения для быстрой локальной итерации (см.
[docs/SETUP.md](docs/SETUP.md)).

Чтобы запустить Go-сервер прямо на хосте, а не в Docker:

```bash
export DATABASE_URL="postgres://ezra:ezra@localhost:5432/ezra?sslmode=disable"
export REDIS_URL="redis://localhost:6379"
make run   # go run ./cmd/ezra
```

Firebase-креды для рабочего локального сервера не нужны — см.
[docs/SETUP.md](docs/SETUP.md) про вход по login/password; Firebase настраивай,
только если тебе конкретно нужно протестировать этот флоу.

## Структура проекта

```
cmd/ezra/       точка входа: собирает все пакеты и запускает HTTP + воркеры
cmd/bot/        headless-рой ботов для ручного теста мультиплеера (не в проде)
internal/       по одному пакету на игровой домен (handler → service → repository)
internal/canon/ центральные игровые константы, общие для всех пакетов
internal/platform/  инфраструктура: Postgres, Redis, Firebase, asynq, Overpass
pkg/            небольшие общие библиотеки (HTTP-хелперы, middleware, гео-математика)
migrations/     SQL-миграции golang-migrate, одна нумерованная пара на изменение
config/         balance.yaml — настраиваемые числа игрового баланса
```

## Документация

Если ты новый человек в проекте — начни отсюда:

1. [docs/DOMAIN_GLOSSARY.md](docs/DOMAIN_GLOSSARY.md) — что такое Rift,
   Tower, Faction, Symbiont и остальное, простым инженерным языком.
2. [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — как устроен код, как
   запрос проходит через систему, фоновые задачи, реалтайм.
3. [docs/API.md](docs/API.md) — весь REST-эндпоинты.
4. [docs/SETUP.md](docs/SETUP.md) — локальное окружение разработки подробно.
5. [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) — как это выкатывается в прод.

`docs/legacy/` — более старые доки из ранней стадии планирования, оставлены
для истории — они описывают систему до того, как была реализована большая
часть текущего функционала, так что при расхождениях верь докам выше, а не им.

## Тесты

```bash
make test   # go test ./...
```

Бизнес-логика покрыта unit-тестами с самописными фейками для
repository/collaborator-интерфейсов (без mock-фреймворков и тестовых
контейнеров). Примеры — в `internal/*/service_test.go`.
