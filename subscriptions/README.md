# Subscriptions API — учёт подписок

Учебный проект: REST API на Go + PostgreSQL для учёта личных подписок
(Spotify, хостинг, онлайн-курсы). Позволяет вести список сервисов,
подписок и платежей, видеть, сколько уходит в месяц и что спишется
в ближайшие 7 дней.

## Стек

- **Go** 1.22+ (используется `http.ServeMux` с шаблонами `{id}`)
- **PostgreSQL** 16 (в Docker)
- **pgx/v5** — драйвер PostgreSQL
- **Adminer** — веб-интерфейс к базе (для отладки)
- Фронт — ванильный HTML/CSS/JS, отдаётся тем же сервером

## Запуск

```bash
# 1. поднять базу и adminer
docker compose up -d

# 2. дождаться статуса healthy
docker compose ps
# db должен быть Up (healthy)

# 3. запустить сервер
go run .
# listening on :8080
```

Открыть:
- **http://localhost:8080** — фронт
- **http://localhost:8081** — Adminer (система: PostgreSQL, сервер: `db`, пользователь: `app`, пароль: `secret`, база: `app`)

Если база не поднялась, `go run .` честно упадёт с `cannot reach database` — так и задумано.

### Пересоздать базу с нуля

```bash
docker compose down -v      # -v удаляет том с данными
docker compose up -d
```

Скрипты `schema.sql` и `seed.sql` применяются **только при первом создании** тома. Если поменяли схему — обязательно `down -v`.

## Переменные окружения

Скопируйте `.env.example` в `.env` (или задайте переменные вручную):

```text
DATABASE_URL=postgres://app:secret@localhost:5432/app?sslmode=disable
PORT=8080
```

Если `.env` нет — используются значения по умолчанию из кода.

> **Важно:** стандартная библиотека Go **не читает `.env` автоматически**.
> Без библиотеки `godotenv` файл `.env` — просто заготовка. Работают
> переменные окружения и fallback-значения.

## Схема данных

| Таблица | Что хранит |
|---|---|
| `users` | пользователи (id, name, email) |
| `services` | каталог сервисов (id, name, monthly_price) |
| `subscriptions` | подписки (user_id, service_id, price, status, started_at, next_payment_at) |
| `payments` | история платежей (subscription_id, amount, paid_at) |

Связи:
- `users → subscriptions` (один ко многим, `ON DELETE CASCADE`)
- `services → subscriptions` (один ко многим, `ON DELETE RESTRICT`)
- `subscriptions → payments` (один ко многим, `ON DELETE CASCADE`)

### Почему `price` хранится в `subscriptions`, а не берётся из `services`

Потому что сервис может поднять цену. У уже подписавшихся цена
**фиксируется в момент подписки** и не меняется, пока они не переподпишутся.
Это стандартная практика в биллинге.

### Почему даты задаёт пользователь, а не сервер

Проект — **трекер подписок**, а не платёжная система. Пользователь
переносит свои реальные подписки, которые уже идут — некоторые
с прошлого года. Дата начала и дата следующего списания — это факты,
которые он знает. Мы ничего не списываем, мы напоминаем.

Сервер валидирует даты, но не переписывает их.

## API

| Метод | Путь | Тело запроса | Успех | Ошибки |
|---|---|---|---|---|
| GET | /health | — | 200 `{"status":"ok"}` | — |
| GET | /services | — | 200 массив | 500 |
| GET | /users/{id}/subscriptions | — | 200 массив | 400, 404 |
| POST | /users/{id}/subscriptions | см. ниже | 201 объект | 400, 404, 422 |
| GET | /users/{id}/upcoming | — | 200 массив | 400, 404 |
| GET | /users/{id}/monthly-total | — | 200 `{"total":...}` | 400, 404 |
| PATCH | /subscriptions/{id} | `{"status":"..."}` | 200 объект | 400, 404, 422 |
| DELETE | /subscriptions/{id} | — | 204 | 400, 404 |
| GET | /subscriptions/{id}/payments | — | 200 массив | 400, 404 |
| POST | /subscriptions/{id}/payments | `{"amount":N}` | 201 объект | 400, 404, 422 |

### Объекты

**Subscription**

```json
{
  "id": 1,
  "user_id": 1,
  "service_id": 2,
  "service_name": "Spotify",
  "price": 199.00,
  "status": "active",
  "started_at": "2024-03-15T00:00:00Z",
  "next_payment_at": "2026-10-15T00:00:00Z"
}
```

**Payment**

```json
{"id": 1, "subscription_id": 1, "amount": 199.00, "paid_at": "2026-09-23T10:00:00Z"}
```

**Service**

```json
{"id": 1, "name": "Spotify", "monthly_price": 199.00}
```

### Создание подписки

Тело `POST /users/{id}/subscriptions` поддерживает **два варианта**:

**Вариант 1 — по `service_id` из каталога:**

```json
{"service_id": 2}
```

**Вариант 2 — по названию (сервис создаётся, если его ещё нет):**

```json
{
  "service_name": "Netflix",
  "price": 599.00,
  "started_at": "2024-03-15T00:00:00Z",
  "next_payment_at": "2026-10-15T00:00:00Z"
}
```

Поля `started_at` и `next_payment_at` **опциональны**. Если не указаны:
- `started_at` = сейчас
- `next_payment_at` = сейчас + 1 месяц

**Если сервис с таким именем уже есть** — используется его цена из каталога,
присланная `price` игнорируется. Это защита от подмены цены.

**Валидация дат:**
- `started_at` не может быть в будущем (с запасом 24 часа на часовые пояса)
- `started_at` не может быть старше 10 лет назад
- `next_payment_at` должен быть позже `started_at`
- `next_payment_at` не может быть дальше 3 лет вперёд

### Статусы подписки

`active` → `paused` → `active` (возобновление)
`active` → `cancelled` (отмена)
`active` → `expired` (срок вышел)

PATCH принимает одно из: `active`, `paused`, `cancelled`, `expired`.

## Примеры curl

```bash
# здоровье
curl.exe -i localhost:8080/health

# каталог сервисов
curl.exe localhost:8080/services

# подписки пользователя
curl.exe localhost:8080/users/1/subscriptions

# сколько уходит в месяц
curl.exe localhost:8080/users/1/monthly-total

# что спишется в ближайшие 7 дней
curl.exe localhost:8080/users/1/upcoming

# создать подписку из каталога
curl.exe -i -X POST localhost:8080/users/1/subscriptions \
  -H 'Content-Type: application/json' \
  -d '{"service_id": 2}'

# создать подписку по имени (сервис создастся, если его нет)
curl.exe -i -X POST localhost:8080/users/1/subscriptions \
  -H 'Content-Type: application/json' \
  -d '{"service_name": "Netflix", "price": 599.00}'

# перенести существующую подписку с датами
curl.exe -i -X POST localhost:8080/users/1/subscriptions \
  -H 'Content-Type: application/json' \
  -d '{"service_name": "Spotify", "price": 199, "started_at": "2024-03-15T00:00:00Z", "next_payment_at": "2026-10-15T00:00:00Z"}'

# поставить на паузу
curl.exe -i -X PATCH localhost:8080/subscriptions/1 \
  -H 'Content-Type: application/json' \
  -d '{"status":"paused"}'

# удалить
curl.exe -i -X DELETE localhost:8080/subscriptions/1

# записать платёж
curl.exe -i -X POST localhost:8080/subscriptions/1/payments \
  -H 'Content-Type: application/json' \
  -d '{"amount": 199.00}'
```

## Тестовые данные

`schema.sql` создаёт схему, `seed.sql` наполняет её:
- 2 пользователя (Аня, Борис)
- 4 сервиса (Spotify, Яндекс Плюс, Хостинг VPS, Онлайн-курсы)
- 5 подписок в разных статусах
- 4 платежа

Чтобы начать с чистых данных:

```bash
docker compose down -v
docker compose up -d
```

## Структура проекта

```text
subscriptions/
├── .env.example        # шаблон конфигурации (коммитим)
├── .env                # реальные значения (в .gitignore)
├── .gitignore
├── docker-compose.yml  # PostgreSQL + Adminer
├── go.mod, go.sum
├── schema.sql          # CREATE TABLE ...
├── seed.sql            # INSERT тестовых данных
├── model.go            # Task, Service, Subscription, Payment
├── store.go            # весь SQL здесь
├── handlers.go         # весь HTTP здесь
├── main.go             # запуск, маршруты, мидлвары
├── static/
│   └── index.html      # фронт
└── README.md
```

## Разделение слоёв

- **`model.go`** — структуры данных. Ничего больше.
- **`store.go`** — весь SQL. Знает про базу, не знает про HTTP.
- **`handlers.go`** — весь HTTP. Знает про `http.Request`, не знает про SQL.
- **`main.go`** — конфигурация, маршруты, мидлвары (CORS, логирование).

Правило: **SQL не течёт в хендлеры, HTTP-коды не текут в store.**
Граница между ними — `ErrNotFound` и другие ошибки, определённые в `store.go`.

---

### Команды для запуска в docer

- docker compose down -v `[Удалить таблицы, контейнеры, структуру и тома]`
- docker compose up -d `[Поднять таблицу]`
- docker compose exec db psql -U app -d app -c "\dt" `[Проверка, что в базе есть таблицы]`
- docker compose ps `[Проверка, что все контейнеры запустились и их состояние]`
- go run . `[Запуск сервера]`
- Ctrl + C `[Остановка сервера]`

### 1. Создать go.mod (если его ещё нет)
- go mod init example.com/subscriptions

### 2. Скачать зависимости, которые импортируются в коде (появляется файл go.sum)
- go get github.com/jackc/pgx/v5/pgxpool

### 3. Привести go.mod и go.sum в порядок
- go mod tidy