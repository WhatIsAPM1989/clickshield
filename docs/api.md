# HTTP API

[Документация](README.md) · [Репозиторий](../README.md)

## Авторизация

| Клиент | Доступ |
|---|---|
| Браузерный tracker | Публичный ключ проекта и разрешённый Origin |
| Администратор | Cookie `clickshield_session`; для изменений — `X-CSRF-Token` |
| Сервер сайта | `OUTCOME_API_TOKEN` только для записи исходов заявок |
| Клиент аналитики / MCP | Приватный analytics Bearer key, чтение одного проекта |

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

Analytics key создаётся в панели и показывается один раз. Его нельзя использовать
для административных изменений или чтения replay. [SDK](../sdk/README.md) и
[MCP](product-analytics.md) используют этот тип ключа.

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
`/v1/analytics/{kind}`. Чтение доступно администратору или ключу аналитики своего
проекта. Настройки replay, целей, инструментов и analytics keys меняет администратор.
Чтение/удаление записей сессий и подключение чтения Метрики требуют admin cookie.

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
