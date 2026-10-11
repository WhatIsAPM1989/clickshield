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
`POST /logout` с cookie и CSRF. Admin-сессия действует 12 часов.

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
clickshield --rescore SESSION_ID [SESSION_ID...]
```
