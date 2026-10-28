# Task Manager API

REST API для управления задачами внутри команд: JWT-аутентификация, ролевая модель,
история изменений, комментарии, кеширование списка задач в Redis, оптимистичная
блокировка и SQL-отчёт по команде.

## Стек

- Go 1.25.11+ (`net/http`, стандартный `ServeMux`)
- MySQL 8.0
- Redis 7
- Docker + Docker Compose
- JWT (HS256), bcrypt
- OpenAPI 3.0.3 + Swagger UI
- Redis cache-aside с TTL 5 минут

## Быстрый запуск

```bash
cp .env.example .env
# замените пароли и JWT_SECRET в .env
docker compose up --build -d
```

Сервис будет доступен на `http://localhost:8080`.

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

- Swagger UI: `http://localhost:8080/docs`
- OpenAPI: `http://localhost:8080/openapi.yaml`
- Логи приложения: `docker compose logs -f app`
- Остановка: `docker compose down`
- Остановка с удалением данных: `docker compose down -v`

`JWT_SECRET` обязателен и должен содержать не менее 32 символов. Пример генерации:

```bash
openssl rand -hex 32
```

PowerShell:

```powershell
-join ((1..32) | ForEach-Object { '{0:x2}' -f (Get-Random -Minimum 0 -Maximum 256) })
```

MySQL, Redis и приложение запускаются в отдельных контейнерах. Приложение ждёт
готовности зависимостей, применяет миграции и только затем начинает принимать трафик.

## Конфигурация

Все параметры читаются из переменных окружения. Секреты в коде не хранятся.

| Переменная | Обязательна | По умолчанию | Назначение |
|---|---:|---|---|
| `HTTP_ADDR` | нет | `:8080` | Адрес HTTP-сервера |
| `MYSQL_DSN` | да | — | DSN MySQL, например `user:pass@tcp(127.0.0.1:3306)/db?parseTime=true&multiStatements=true` |
| `REDIS_ADDR` | да | — | Адрес Redis, например `127.0.0.1:6379` |
| `REDIS_PASSWORD` | нет | пусто | Пароль Redis |
| `REDIS_DB` | нет | `0` | Номер БД Redis |
| `JWT_SECRET` | да | — | Секрет подписи JWT, минимум 32 символа |
| `JWT_ISSUER` | нет | `taskmanager` | Issuer JWT |
| `JWT_TTL` | нет | `24h` | Время жизни JWT |
| `TASK_CACHE_TTL` | нет | `5m` | TTL кеша списка задач |
| `AUTO_MIGRATE` | нет | `true` | Применять миграции при старте |
| `LOG_LEVEL` | нет | `info` | `debug`, `info`, `warn`, `error` |
| `SHUTDOWN_TIMEOUT` | нет | `15s` | Graceful shutdown |

Приложение само выставляет UTC-сессию MySQL (`time_zone = '+00:00'`), использует
`parseTime=true` и включает `multiStatements=true` для применения миграций.
В DSN пароль со спецсимволами должен быть URL-экранирован.

## Docker Compose

`docker-compose.yml` поднимает:

- `mysql` — MySQL 8.0, volume `mysql_data`;
- `redis` — Redis 7 с AOF, volume `redis_data`;
- `app` — собранный Go-бинарник, non-root пользователь, healthcheck.

Порты можно изменить через `HTTP_PORT` и `MYSQL_PORT` в `.env`.
Пароль приложения и root-пароль MySQL задаются в `.env`; compose не содержит
рабочих секретов.

## Миграции

SQL-миграции лежат в [`internal/db/migrations/`](internal/db/migrations) и
встроены в бинарник через `go:embed`. Движок находится в
[`internal/db/migrate.go`](internal/db/migrate.go); приложение вызывает его через
`db.Migrate(ctx, database)`. Пути в директиве `go:embed` относительны этого
файла, поэтому SQL-миграции лежат рядом с ним.

Файлы именуются по правилам golang-migrate:

```text
internal/db/migrations/
  001_init.up.sql
  001_init.down.sql
```

При `AUTO_MIGRATE=true` (по умолчанию) миграции применяются при старте до
начала приёма трафика. Состояние хранится в таблице `schema_migrations`
(`version`, `dirty`); уже применённые версии пропускаются, поэтому повторный
запуск безопасен. Если миграция завершилась ошибкой, база помечается как
`dirty` и требует ручного вмешательства.

Миграционные файлы содержат несколько DDL-операторов, поэтому для соединения
включён `multiStatements=true`. `db.Open` выставляет его автоматически; в
примерах DSN ниже он указан явно.

Основные таблицы: `users`, `teams`, `team_members`, `tasks`, `task_history`,
`task_comments`.

Схема дополнительно защищает инварианты на уровне БД:

- уникальные `users.email` и пары `team_members(team_id, user_id)`;
- исполнитель и создатель задачи должны быть участниками её команды
  (составные внешние ключи);
- `tasks.assignee_id` может быть `NULL` (задача без исполнителя);
- индексы под фильтры задач, историю и отчёты.

## Роли и права

| Действие | owner | admin | member |
|---|:---:|:---:|:---:|
| Приглашать участников | да | да, только `member` | нет |
| Менять роли | да | нет | нет |
| Изменить роль owner | нет | нет | нет |
| Редактировать любую задачу команды | да | да | нет |
| Создавать задачи | да | да | да |
| Редактировать свою задачу | да | да | да |
| Менять только статус назначенной задачи | да | да | да |
| Видеть историю и комментарии | да | да | да |
| Смотреть аналитику `/stats` | да | да | нет |

Дополнительно:

- Создатель задачи может менять `title`, `description`, `status`, `assignee_id`.
- Исполнитель (если он не создатель и не admin/owner) может менять только
  `status`; переназначение и изменение текста запрещены.
- Обычный участник не может редактировать чужую задачу, если он не её создатель
  и не исполнитель.
- Исполнитель обязан быть участником той же команды.
- Роль `owner` не выдаётся через приглашение и не меняется через API; владелец
  команды фиксируется при создании.
- Пользователь не видит чужие команды. Для чужих задач/комментариев/истории
  возвращается `404`, чтобы не раскрывать их существование.

## API

Базовый префикс: `/api/v1`. Защищённые эндпоинты требуют:

```http
Authorization: Bearer <JWT>
```

### Эндпоинты

| Метод | Путь | Доступ |
|---|---|---|
| `POST` | `/api/v1/register` | публичный |
| `POST` | `/api/v1/login` | публичный |
| `POST` | `/api/v1/teams` | авторизованный |
| `GET` | `/api/v1/teams` | авторизованный |
| `GET` | `/api/v1/teams/{id}/members` | участник команды |
| `POST` | `/api/v1/teams/{id}/invite` | owner/admin |
| `PATCH` | `/api/v1/teams/{id}/members/{user_id}` | owner |
| `GET` | `/api/v1/teams/{id}/stats` | owner/admin |
| `POST` | `/api/v1/tasks` | участник команды |
| `GET` | `/api/v1/tasks?team_id=...` | участник команды |
| `GET` | `/api/v1/tasks/{id}` | участник команды |
| `PUT` | `/api/v1/tasks/{id}` | по правилам редактирования |
| `GET` | `/api/v1/tasks/{id}/history` | участник команды |
| `POST` | `/api/v1/tasks/{id}/comments` | участник команды |
| `GET` | `/api/v1/tasks/{id}/comments` | участник команды |
| `GET` | `/healthz` | публичный |
| `GET` | `/readyz` | публичный |

Статусы задач: `todo`, `in_progress`, `done`. При переходе в `done`
устанавливается `closed_at`; при переоткрытии — очищается.

### Ошибки

Единый формат:

```json
{
  "error": {
    "code": "version_conflict",
    "message": "task was changed by another request",
    "details": {"current_version": 4},
    "request_id": "8f7a4f3d2c1b"
  }
}
```

Коды: `validation_error`, `unauthorized`, `forbidden`, `not_found`,
`version_conflict`, `email_taken`, `already_member`, `invalid_assignee`,
`internal_error` и др. `X-Request-ID` возвращается в каждом ответе.

## Примеры запросов

### Регистрация и логин

```bash
curl -s -X POST http://localhost:8080/api/v1/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"secret-password","name":"Alice"}'

curl -s -X POST http://localhost:8080/api/v1/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"secret-password"}'
```

Из ответа логина возьмите `token`:

```bash
TOKEN='<token>'
```

### Команда, участник и задача

```bash
# Создать команду (текущий пользователь станет owner)
curl -s -X POST http://localhost:8080/api/v1/teams \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Platform"}'

# Пригласить пользователя с id=2
curl -s -X POST http://localhost:8080/api/v1/teams/1/invite \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"user_id":2,"role":"member"}'

# Создать задачу
curl -s -X POST http://localhost:8080/api/v1/tasks \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"team_id":1,"title":"Fix login timeout","description":"Investigate Redis TTL","assignee_id":2}'
```

### Список задач с фильтрами

```bash
curl -s -G http://localhost:8080/api/v1/tasks \
  -H "Authorization: Bearer $TOKEN" \
  --data-urlencode 'team_id=1' \
  --data-urlencode 'status=todo' \
  --data-urlencode 'assignee_id=2' \
  --data-urlencode 'limit=20' \
  --data-urlencode 'offset=0'
```

`team_id` обязателен. `limit` — от 1 до 100 (по умолчанию 20), `offset` — от 0.
Проверка участия выполняется до чтения из кеша.

### Обновление с оптимистичной блокировкой

`version` обязателен и равен версии из последнего чтения. Успешное изменение
увеличивает версию на 1. Устаревшая версия возвращает `409 version_conflict`.

```bash
curl -s -X PUT http://localhost:8080/api/v1/tasks/1 \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"version":1,"status":"done"}'
```

Чтобы снять исполнителя, явно передайте `"assignee_id": null`.
Если поле не передано, оно не меняется.

### История и комментарии

```bash
curl -s http://localhost:8080/api/v1/tasks/1/history \
  -H "Authorization: Bearer $TOKEN"

curl -s -X POST http://localhost:8080/api/v1/tasks/1/comments \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"content":"Воспроизвёл на staging"}'

curl -s http://localhost:8080/api/v1/tasks/1/comments \
  -H "Authorization: Bearer $TOKEN"
```

Каждая запись истории содержит `changed_by`, `created_at` и JSON `changes` с
парами `old`/`new`. Создание задачи также записывается в историю.

### SQL-отчёт

```bash
curl -s http://localhost:8080/api/v1/teams/1/stats \
  -H "Authorization: Bearer $TOKEN"
```

Пример ответа:

```json
{
  "team_id": 1,
  "generated_at": "2026-10-27T12:00:00Z",
  "total_tasks": 12,
  "tasks_by_status": {"todo": 5, "in_progress": 2, "done": 5},
  "top_assignees_30d": [
    {"user_id": 2, "name": "Bob", "closed_tasks_30d": 3},
    {"user_id": 5, "name": "Eve", "closed_tasks_30d": 2},
    {"user_id": 7, "name": "Sam", "closed_tasks_30d": 1}
  ],
  "average_close_time_hours": 18.75,
  "comments_by_task": [
    {"task_id": 4, "title": "Deploy", "comment_count": 6},
    {"task_id": 1, "title": "Fix login", "comment_count": 2}
  ]
}
```

Отчёт строится **одним SQL-запросом с CTE** (`internal/store/stats.go`):
`JOIN`, `GROUP BY`, `COUNT`, `AVG`, `TIMESTAMPDIFF`, `INTERVAL 30 DAY`.
Каждый CTE фильтруется по `team_id`. Доступ проверяется до выполнения запроса:
только owner/admin команды; N+1 отсутствует.

## Кеш Redis

- Кешируется только страница списка задач.
- Ключ включает `team_id`, версию инвалидации команды, `status`, `assignee_id`,
  `limit`, `offset`.
- TTL — 5 минут (`TASK_CACHE_TTL`).
- После создания/изменения задачи версия команды инкрементируется (`INCR`).
  Старые ключи больше не читаются и истекают по TTL. Это инвалидирует все
  фильтры команды одной атомарной операцией без `SCAN`/`KEYS`.
- Проверка прав выполняется всегда, независимо от кеша.
- При недоступности Redis API продолжает работать через MySQL; ошибки кеша
  логируются, но не ломают запрос. `/readyz` при этом вернёт `503`.

## Транзакции и конкурентность

- Создание команды + membership owner — одна транзакция.
- Приглашение участника (проверка прав, пользователя и вставка) — одна транзакция.
- Создание задачи + первая запись истории — одна транзакция.
- Обновление задачи + запись истории — одна транзакция.
- `UPDATE tasks` защищён `WHERE version = ?`; перед изменением строка читается
  `SELECT ... FOR UPDATE`. Конкурентный запрос со старой версией получает
  `409`, история не расходится с актуальным состоянием.
- История строится от состояния, прочитанного под блокировкой.

## Тестирование

Unit-тесты бизнес-логики прав:

```bash
go test ./...
```

Обязательный интеграционный тест SQL-отчёта находится в
[`integration/stats_test.go`](integration/stats_test.go). Ему нужен MySQL 8.

Самый простой способ — профиль `test` в compose:

```bash
cp .env.example .env
docker compose --profile test run --rm --build integration
```

Профиль собирает Go-стадию образа, дожидается MySQL и запускает
`go test -v ./integration -run TestTeamStatsSQL`. Тест создаёт уникальные строки
и удаляет их после себя.

Альтернативно можно создать отдельную БД и запустить тест с хоста:

```bash
docker compose exec -T mysql sh -c \
  'mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -e "CREATE DATABASE IF NOT EXISTS taskmanager_test CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"'

TEST_MYSQL_DSN='root:<root-password>@tcp(127.0.0.1:3306)/taskmanager_test?parseTime=true&multiStatements=true' \
  go test -v ./integration -run TestTeamStatsSQL
```

PowerShell:

```powershell
$env:TEST_MYSQL_DSN = 'root:<root-password>@tcp(127.0.0.1:3306)/taskmanager_test?parseTime=true&multiStatements=true'
go test -v ./integration -run TestTeamStatsSQL
```

Тест:

- применяет миграции;
- создаёт две команды и данные, включая закрытые задачи внутри и вне 30-дневного
  окна;
- проверяет статусы, топ-3 исполнителей, среднее время закрытия и комментарии;
- проверяет, что данные второй команды не попадают в отчёт;
- проверяет доступ owner/admin/member/чужака;
- удаляет созданные строки после завершения.

Без `TEST_MYSQL_DSN` интеграционный тест пропускается, поэтому для
`go test ./...` не нужен локальный MySQL.

## Структура проекта

```text
cmd/api/                  точка входа, graceful shutdown
internal/api/             HTTP-роутер, middleware, DTO, handlers
internal/auth/            JWT и bcrypt
internal/cache/           Redis cache-aside
internal/config/          конфигурация из env
internal/db/              подключение к MySQL и мигратор (db.Migrate)
internal/db/migrations/   встроенные SQL-миграции (up/down)
internal/model/           доменные модели
internal/service/         бизнес-логика и проверки прав
internal/store/           SQL-репозитории, транзакции, SQL-отчёт
docs/openapi.yaml         OpenAPI 3.0.3
integration/stats_test.go интеграционный тест отчёта
```

## Локальный запуск без Docker

Docker не обязателен: нужны только Go, MySQL и Redis. Все зависимости
подключаются через переменные окружения.

### 1. Создать базу MySQL

Мигратор создаёт таблицы, но не саму базу. Подключитесь к MySQL и выполните:

```sql
CREATE DATABASE IF NOT EXISTS taskmanager
  CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

При необходимости создайте отдельного пользователя:

```sql
CREATE USER IF NOT EXISTS 'taskapp'@'localhost' IDENTIFIED BY 'password';
GRANT ALL PRIVILEGES ON taskmanager.* TO 'taskapp'@'localhost';
FLUSH PRIVILEGES;
```

Для применения миграций у пользователя должны быть права на `CREATE`, `ALTER`,
`INDEX` и `DROP` (для down-миграций). При `AUTO_MIGRATE=false` приложение не
трогает схему; миграции нужно применить вручную, например выполнив файлы
`internal/db/migrations/*.up.sql` по порядку.

### 2. Запустить Redis

```bash
redis-server --port 6379
```

На Windows можно использовать Memurai, Redis через WSL или любой совместимый
сервер. Redis используется для кеша списков задач; при его недоступности
приложение продолжит работать с MySQL и будет периодически пытаться восстановить
соединение.

### 3. Задать окружение и запустить приложение

**PowerShell (Windows):**

```powershell
$env:MYSQL_DSN='taskapp:password@tcp(127.0.0.1:3306)/taskmanager?parseTime=true&multiStatements=true'
$env:REDIS_ADDR='127.0.0.1:6379'
$env:JWT_SECRET='<случайная строка не короче 32 символов>'
$env:HTTP_ADDR=':8080'
go run ./cmd/api
```

Секрет можно сгенерировать так:

```powershell
-join ((1..32) | ForEach-Object { '{0:x2}' -f (Get-Random -Minimum 0 -Maximum 256) })
```

**Linux/macOS:**

```bash
export MYSQL_DSN='taskapp:password@tcp(127.0.0.1:3306)/taskmanager?parseTime=true&multiStatements=true'
export REDIS_ADDR='127.0.0.1:6379'
export JWT_SECRET="$(openssl rand -hex 32)"
export HTTP_ADDR=':8080'
go run ./cmd/api
```

Проверка после запуска:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
```

Swagger UI: `http://127.0.0.1:8080/docs`, OpenAPI-документ:
`http://127.0.0.1:8080/openapi.yaml`.

Для сборки без запуска:

```bash
go build -o bin/taskmanager ./cmd/api
./bin/taskmanager
```

## Примечания

- Пароли хранятся только как bcrypt-хеши и никогда не возвращаются API.
- JWT подписывается HS256, проверяются issuer, срок действия и метод подписи.
- Тело запроса ограничено 1 MiB, неизвестные JSON-поля отклоняются.
- Swagger UI использует CDN; сам OpenAPI-документ всегда доступен локально по
  `/openapi.yaml`.
- Логи структурированные (JSON, `slog`) и содержат `request_id`.
