# CommentTree

Сервис древовидных комментариев на Go с неограниченной глубиной вложенности. Поддерживает создание и каскадное удаление комментариев, полнотекстовый поиск, сортировку и постраничную навигацию. Для хранения данных используется PostgreSQL, взаимодействие с сервисом доступно через REST API и веб-интерфейс.

## Содержание

- [Функциональность](#функциональность)
- [Стек технологий](#стек-технологий)
- [Структура проекта](#структура-проекта)
- [Запуск через Docker Compose](#запуск-через-docker-compose)
- [HTTP API](#http-api)
- [Команды Makefile и тестирование](#команды-makefile-и-тестирование)

## Функциональность

- Создание комментариев и ответов с именем автора.
- Просмотр дерева и каскадное удаление поддеревьев.
- Вложенность без фиксированного ограничения уровня.
- Курсорная пагинация корней с полными поддеревьями, сортировка от новых к старым и наоборот.
- Полнотекстовый поиск на русском и английском языках с отдельной пагинацией.
- Переход из поиска к обсуждению с выделением найденного комментария.
- Веб-интерфейс для просмотра, создания, удаления и поиска комментариев.

## Стек технологий

- Go, chi, slog.
- PostgreSQL, pgx/v5, Goose.
- HTML, CSS, Vanilla JS.
- Docker, Docker Compose, Make.
- Testcontainers.

## Структура проекта

```text
.
├── cmd/server/main.go       # Точка входа
├── configs/config.yaml      # Настройки приложения
├── internal/
│   ├── app/                 # Сборка зависимостей и жизненный цикл
│   ├── config/              # Загрузка конфигурации
│   ├── domain/              # Модель и доменные правила
│   ├── service/comments/    # Сервисный слой
│   ├── repository/comments/ # Работа с комментариями в БД
│   ├── infra/postgres/      # Подключение к PostgreSQL
│   ├── delivery/http/       # HTTP API и middleware
│   └── logger/              # Настройка логирования
├── docker/postgres/         # Настройка ролей и прав БД
├── migrations/              # SQL-миграции Goose
├── tests/
│   ├── integration/         # Интеграционные тесты
│   └── testutil/            # Инфраструктура тестов
├── web/                     # Веб-интерфейс
├── .env.example             # Пример переменных окружения
├── .dockerignore            # Исключения сборки образов
├── Dockerfile               # Образы приложения и миграций
├── docker-compose.yml       # Сервисы Docker Compose
├── Makefile                 # Команды разработки и проверок
└── go.mod, go.sum           # Зависимости Go
```

## Запуск через Docker Compose

Нужны Git, Docker с Docker Compose и Make.

```sh
git clone https://github.com/akhmed9505/comment-tree.git
cd comment-tree
cp .env.example .env
```

Задайте в `.env` три разные роли и их пароли:

- `POSTGRES_USER` / `POSTGRES_PASSWORD` — администратор инициализации БД.
- `MIGRATION_USER` / `MIGRATION_PASSWORD` — владелец объектов проекта и исполнитель миграций.
- `APP_DB_USER` / `APP_DB_PASSWORD` — приложение: чтение, создание и удаление комментариев.

Для существующего тома сохраните прежние `POSTGRES_DB`, `POSTGRES_USER` и `POSTGRES_PASSWORD`. При запуске роли и права настраиваются повторно, объекты проекта передаются роли миграций; данные сохраняются. `APP_DATABASE_URL` для локального запуска должен содержать учётные данные роли приложения.

В Compose приложению передаются только его учётные данные и явно перечисленные настройки `APP_*` из `.env`. Неуказанные настройки берутся из YAML и значений по умолчанию; адрес HTTP внутри контейнера и подключение к сервису PostgreSQL задаёт Compose.

Запуск:

```sh
make docker-up
```

Остановка:

```sh
make docker-down
```

## HTTP API

**Создать комментарий**

```sh
curl -i "http://localhost:8080/comments" \
  -H 'Content-Type: application/json' \
  -d '{"author":"Анна","body":"Обсудим поиск","parent_id":null}'
```

Ответ — `201 Created`:

```json
{"id":"1","parent_id":null,"author":"Анна","body":"Обсудим поиск","created_at":"2026-11-19T12:00:00Z"}
```

**Ответить на комментарий**

```sh
curl -i "http://localhost:8080/comments" \
  -H 'Content-Type: application/json' \
  -d '{"author":"Михаил","body":"Поиск работает","parent_id":"1"}'
```

Ответ — `201 Created`:

```json
{"id":"2","parent_id":"1","author":"Михаил","body":"Поиск работает","created_at":"2026-11-19T12:01:00Z"}
```

**Получить комментарии или поддерево**

```sh
curl "http://localhost:8080/comments?limit=20&sort=newest"
curl "http://localhost:8080/comments?limit=20&sort=oldest"
curl "http://localhost:8080/comments?parent=1"
```

Ответ для обсуждения из примера — `200 OK`:

```json
{
  "comments": [
    {"id":"1","parent_id":null,"author":"Анна","body":"Обсудим поиск","created_at":"2026-11-19T12:00:00Z"},
    {"id":"2","parent_id":"1","author":"Михаил","body":"Поиск работает","created_at":"2026-11-19T12:01:00Z"}
  ],
  "next_cursor": null
}
```

Если ответ содержит непустой `next_cursor`, следующая страница запрашивается с теми же `limit` и `sort`:

```sh
CURSOR='значение next_cursor из ответа'
curl -G "http://localhost:8080/comments" \
  --data-urlencode 'limit=20' --data-urlencode 'sort=newest' \
  --data-urlencode "cursor=$CURSOR"
```

**Найти комментарии**

```sh
curl -G "http://localhost:8080/comments/search" \
  --data-urlencode 'q=работает' --data-urlencode 'limit=20'
```

Ответ — `200 OK`:

```json
{
  "matches": [
    {
      "comment": {"id":"2","parent_id":"1","author":"Михаил","body":"Поиск работает","created_at":"2026-11-19T12:01:00Z"},
      "root_id": "1"
    }
  ],
  "next_cursor": null
}
```

Для следующей страницы поиска передайте `cursor` из ответа с прежним `q`:

```sh
CURSOR='значение next_cursor из ответа поиска'
curl -G "http://localhost:8080/comments/search" \
  --data-urlencode 'q=работает' --data-urlencode 'limit=20' \
  --data-urlencode "cursor=$CURSOR"
```

**Удалить поддерево**

```sh
curl -i -X DELETE "http://localhost:8080/comments/1"
```

Ответ — `204 No Content`, без тела.

**Проверить состояние сервиса**

```sh
curl "http://localhost:8080/healthz"
curl "http://localhost:8080/readyz"
```

Ответ каждого endpoint при готовности сервиса — `200 OK`:

```json
{"status":"ok"}
```

**Пример ошибки: отсутствующий комментарий**

```sh
curl -i "http://localhost:8080/comments?parent=1"
```

После удаления ответ — `404 Not Found`:

```json
{"error":{"code":"comment_not_found","message":"Comment not found"}}
```

## Команды Makefile и тестирование

Для запуска тестов необходимы Go 1.27+, Make, Docker и установленный Goose CLI:

```sh
go install github.com/pressly/goose/v3/cmd/goose@v3.27.0
make check
```

- `make run` — локальный запуск Go-процесса.
- `make fmt` — форматирование Go-кода.
- `make vet` — статические проверки.
- `make test` — unit-тесты с race detector.
- `make test-integration` — все тесты, включая интеграционные с Testcontainers и race detector.
- `make check` — проверка форматирования, vet, тесты и сборка.
- `make docker-up` — сборка и запуск Docker Compose.
- `make docker-down` — остановка Docker Compose с сохранением данных.
