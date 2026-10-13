# HTTP API

[Документация](README.md) · [Репозиторий](../README.md)

## Авторизация

| Клиент | Доступ |
|---|---|
| Браузерный tracker | Публичный ключ проекта и разрешённый Origin |
| Администратор | Cookie `clickshield_session`; для изменений — `X-CSRF-Token` |
| Сервер сайта | `OUTCOME_API_TOKEN` только для записи исходов заявок |
| Скрипт, SDK, ИИ-агент (MCP) | API-токен `Authorization: Bearer cst_…`: права и один или все проекты |

Вход: `POST /login` с `application/x-www-form-urlencoded`, полями `username` и
`password`. Ответ перенаправляет на `/` при успехе либо `/login?error=1` при неверных
данных. Сохраните cookie, проверьте `GET /v1/me` и возьмите `csrf_token` из ответа;
сам по себе HTTP 200 после редиректа не доказывает успешный вход. Выход:
`POST /logout` с cookie и CSRF. Admin-сессия действует 12 часов, а с полем
`remember=1` («Запомнить меня») — 90 дней.

### Пользователи и приглашения

Главный администратор — пользователь из `ADMIN_USERNAME` (`GET /v1/me` возвращает
`is_owner: true`). Только он управляет доступом, остальным эти маршруты отвечают 403:

```text
GET    /v1/invites              активные неиспользованные ссылки
POST   /v1/invites              создать ссылку: {path, expires_at}; путь показывается один раз
DELETE /v1/invites/{id}         отозвать ссылку
GET    /v1/users                все пользователи
POST   /v1/users/{id}/active    {"active": false|true}; отключение завершает сессии
```

Ссылка `/invite/{token}` одноразовая и действует 7 дней. `POST /invite/{token}`
с полями `email`, `password`, `password2` (пароль от 12 символов) создаёт
пользователя с логином = email и открывает сессию на 12 часов. Ошибки возвращаются
редиректом на `/invite/{token}?error=invalid|email|short|mismatch|exists`.

### API-токены

Токен выпускает любой администратор в панели, раздел «API и MCP». Он показывается
один раз, в БД хранится SHA-256. Токен привязан к одному проекту или ко всем,
действует 1–365 дней и перестаёт работать после отзыва или отключения выпустившего
пользователя. Ключи `csak_…` прежних версий продолжают работать с правом `analytics:read`.

| Право | Маршруты |
|---|---|
| `analytics:read` | `GET /v1/analytics/{kind}`, `/v1/traffic`, `/v1/reports`, `/v1/reports/timeline`, `GET /v1/projects/{id}/definitions`, `GET /v1/projects/{id}/tools/{kind}` |
| `analytics:write` | `POST/PUT/DELETE` целей, воронок, ссылок, пикселей и отметок |
| `sessions:read` | `GET /v1/sessions`, `/v1/sessions/export.csv`, `/v1/sessions/{id}`, `/v1/stats`, `/v1/bots`, `/v1/quality` |
| `sessions:write` | `POST /v1/sessions/{id}/label`, `/v1/sessions/{id}/outcomes` |
| `replay:read`, `replay:write` | `GET`, `DELETE /v1/sessions/{id}/replay` |
| `projects:read` | `GET /v1/projects`, `/v1/projects/{id}/integrations`, `/metrika-read`, `/metrika-report`, `/v1/outbox`, `/v1/operations` |
| `projects:write` | `POST /v1/projects`, `PUT /v1/projects/{id}`, `/replay`, `/integrations/{provider}`, `POST …/test`, `PUT/DELETE /metrika-read` |

Право записи включает чтение той же области. Без нужного права — 403, без
действующего токена — 401. С токеном CSRF-заголовок не нужен. Токен одного проекта:
`project_id` в запросе можно опустить, чужой проект — 403, чужая сессия — 404;
`/v1/outbox`, `/v1/operations` и создание проекта доступны только токену всех
проектов. `GET /v1/projects` возвращает токену только его проект. Изменения через
токен пишутся в аудит (`API_TOKEN_REQUEST`) от имени выпустившего пользователя.

Пользователи, приглашения, `/v1/me` и сами токены доступны только из панели:

```text
GET    /v1/api-tokens           список (без секретов) и перечень прав
POST   /v1/api-tokens           {name, project_id|"", scopes[], days}; ответ {token, raw}, raw показывается один раз
DELETE /v1/api-tokens/{id}      отозвать
```

[SDK](../sdk/README.md) и [MCP](mcp.md) используют эти токены.

## Основные маршруты

Публичные tracker endpoints используют `?key=PUBLIC_PROJECT_KEY` и проверяют Origin проекта:

```text
POST /v1/events
POST /v1/session/finish
GET  /v1/tracker/config
POST /v1/replay
```

Серверный endpoint сайта авторизуется заголовком `Authorization: Bearer OUTCOME_API_TOKEN`:

```text
POST /v1/outcomes
```

Административные endpoints требуют cookie-сессию и CSRF-токен для изменений:

```text
GET  /v1/sessions
GET  /v1/sessions/export.csv
GET  /v1/sessions/{id}
POST /v1/sessions/{id}/label
POST /v1/sessions/{id}/outcomes
GET  /v1/stats
GET  /v1/bots
GET  /v1/quality
GET  /v1/operations

GET  /v1/projects
POST /v1/projects
PUT  /v1/projects/{id}
GET  /v1/projects/{id}/integrations
PUT  /v1/projects/{id}/integrations/{provider}
POST /v1/projects/{id}/integrations/{provider}/test
GET  /v1/outbox
```

Отчёты: `GET /v1/traffic`, `/v1/reports`, `/v1/reports/timeline`,
`/v1/analytics/{kind}`. Доступны администратору или токену с `analytics:read`.

`from` включительно, `to` исключительно; передавайте даты с часовым поясом,
а для календарных группировок — `timezone`. Фильтры и схемы ответов описаны в OpenAPI.

Полное описание: [OpenAPI](openapi.yaml).

Пересчитать отдельные сессии с текущими правилами можно без запуска HTTP-сервера:

```bash
clickshield rescore SESSION_ID [SESSION_ID...]
# Воспроизвести сохранённые входы и правила конкретной автоматической оценки:
clickshield reproduce CLASSIFICATION_ID
```

Отчёты и `/v1/stats` читают проекции `analytics` и могут отставать от записи
сессий. `/v1/operations` показывает `analytics_pending`, `analytics_lag_seconds`,
`analytics_retries`, `replay_gc_failed`; остальные поля сохранены. HTTP и MCP
используют общий сценарий отчётов и одну проверку параметров.

`PUT /v1/projects/{id}/replay` возвращает 503 при попытке включить запись без S3.
