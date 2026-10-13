# MCP для ИИ-агентов

[Документация](README.md) · [HTTP API](api.md) · [OpenAPI](openapi.yaml)

ClickShield предоставляет MCP-сервер (Streamable HTTP, без сохраняемых сессий,
ответы JSON) по адресу `https://clickshield.example.com/mcp`. Он включён по
умолчанию (`MCP_ENABLED=true`). Доступ — только по API-токену из раздела панели
«API и MCP»; cookies не принимаются, Origin браузера должен совпадать с хостом.

## Подключение

1. Панель → «API и MCP» → «Новый токен»: название, проект или «Все проекты»,
   права, срок. Скопируйте токен — он показывается один раз.
2. Подключите клиента. Раздел «Подключение» показывает команды с уже подставленным токеном.

Claude Code:

```bash
claude mcp add --transport http clickshield https://clickshield.example.com/mcp \
  --header "Authorization: Bearer cst_..."
```

Cursor, Claude Desktop и другие клиенты с JSON-конфигом:

```json
{"mcpServers": {"clickshield": {"type": "http", "url": "https://clickshield.example.com/mcp",
  "headers": {"Authorization": "Bearer cst_..."}}}}
```

## Инструменты

Агент видит только инструменты, разрешённые правами токена: `tools/list`
отличается для разных токенов и отдаётся с `cacheScope: private`. Инструменты
вызывают те же маршруты HTTP API внутри сервера, поэтому права, привязка к проекту,
проверка параметров и аудит одинаковы. Ошибка (неверный фильтр, чужая сессия)
возвращается как результат инструмента с `isError`, чтобы агент мог исправить запрос.

| Право | Инструменты |
|---|---|
| любое | `whoami` — имя токена, проект, права |
| `analytics:read` | `analytics_report` (traffic, отчёты по источникам/кампаниям/UTM/устройствам/гео…, events, properties, goals, funnels, journey, retention, heatmap, links, pixels, annotations), `report_timeline`, `list_definitions`, `list_campaign_tools` |
| `analytics:write` | `save_definition`, `delete_definition`, `save_campaign_tool`, `delete_campaign_tool` |
| `sessions:read` | `list_sessions`, `get_session`, `get_stats`, `get_bot_stats`, `get_lead_quality` |
| `sessions:write` | `label_session`, `set_lead_outcome` |
| `replay:read` / `replay:write` | `get_session_replay` / `delete_session_replay` |
| `projects:read` | `list_projects`, `list_integrations`, `get_metrika_report`, `get_operations` |
| `projects:write` | `create_project`, `update_project`, `update_replay_settings`, `update_integration`, `test_integration`, `set_metrika_connection`, `delete_metrika_connection` |

Фильтры отчётов и сессий: `project_id`, `from`/`to` (RFC3339 с часовым поясом,
`to` исключительно), `classification`, `source`, `channel`, `campaign`, `device`,
`landing_url`, `bot`, `bot_category`, `paid_only` и `dimensions` (utm_*, referrer,
страна, браузер и др.). Токен одного проекта подставляет проект сам; токену всех
проектов для настроек проекта нужен `project_id` (см. `list_projects`).
Результат больше 1 МБ отклоняется — сузьте период или фильтры; большие записи
сессий смотрите в панели.

## Безопасность

- Выдавайте минимальные права. `sessions:read` открывает IP, fingerprint и
  рекламные идентификаторы посетителей.
- Инструменты записи меняют рабочие данные: разметка сессий и исходы заявок уходят
  в Метрику, GA4 и VK Рекламу, режим `BLOCKING` влияет на реальных посетителей.
  Аннотации MCP помечают такие инструменты (`destructiveHint`, `openWorldHint`),
  а инструкции сервера просят агента подтверждать изменения у пользователя.
- Секреты интеграций можно записать, но они никогда не возвращаются.
- Пользователи, приглашения и токены недоступны через MCP и API-токены.
- Отзовите токен в панели, если он больше не нужен или мог утечь.
