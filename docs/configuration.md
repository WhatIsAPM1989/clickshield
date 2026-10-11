# Настройка ClickShield

[Документация](README.md) · [Репозиторий](../README.md)

Переменные читает `internal/config/config.go`. Docker Compose передаёт их явно;
при запуске бинарника `.env` автоматически не загружается. Значения для Compose —
в [`.env.example`](../.env.example).

## Переменные окружения

| Переменная | Назначение | По умолчанию |
|---|---|---|
| `HTTP_ADDR` | адрес HTTP-сервера | `:8080` |
| `DATABASE_URL` | PostgreSQL DSN | обязательно |
| `POSTGRES_PASSWORD` | пароль PostgreSQL в Docker Compose | обязательно |
| `ADMIN_USERNAME` | логин администратора | `admin` |
| `ADMIN_PASSWORD` | пароль, минимум 12 символов | обязательно |
| `SESSION_SECRET` | HMAC-ключ серверных admin-сессий, минимум 32 байта | обязательно |
| `ENCRYPTION_KEY` | base64 от 32 байт для AES-GCM | обязательно |
| `COOKIE_SECURE` | cookie только по HTTPS | `false` в dev |
| `TRUST_PROXY_HEADERS` | доверять `CF-Connecting-IP` / `X-Forwarded-For` | `false` |
| `BOOTSTRAP_PROJECT_NAME` | имя первого проекта | `Demo site` |
| `BOOTSTRAP_PROJECT_KEY` | публичный ключ первого проекта | генерируется, если пусто |
| `BOOTSTRAP_ALLOWED_ORIGINS` | разрешённые origins через запятую | пусто |
| `YANDEX_COUNTER_ID` | bootstrap Yandex integration | пусто |
| `YANDEX_MEASUREMENT_TOKEN` | bootstrap Yandex secret | пусто |
| `GOOGLE_MEASUREMENT_ID` | bootstrap GA4 integration | пусто |
| `GOOGLE_API_SECRET` | bootstrap GA4 secret | пусто |
| `OUTCOME_API_TOKEN` | Bearer-токен для `POST /v1/outcomes` | пусто (endpoint отключён) |
| `ALERT_WEBHOOK_URL` | адрес JSON-уведомлений о проблемах | пусто |
| `RAW_EVENTS_TTL` | хранение raw events | `168h` |
| `AGGREGATE_TTL` | хранение sessions/features/classifications | `4320h` |
| `STALE_SESSION_AFTER` | автозавершение потерянной сессии | `30m` |
| `FINISH_GRACE` | пауза перед scoring после pagehide | `20s` |
| `GEOIP_CITY_DATABASE` | путь к локальной City MMDB; неверный заданный путь останавливает старт | пусто |
| `MCP_ENABLED` | включить read-only endpoint `/mcp` | `false` |
| `ASN_DATABASE` | база IP→ASN в формате iptoasn (`.tsv.gz`) | `/app/ip2asn.tsv.gz` |

`TRUST_PROXY_HEADERS=true` допустим только за доверенным proxy/CDN, который очищает входящие заголовки. ASN берётся из `X-GeoIP-ASN`/`CF-ASN`, организация — из `X-GeoIP-Organization`. Если proxy их не передаёт, ASN и организация определяются по базе [iptoasn.com](https://iptoasn.com) (public domain). Docker-образ скачивает её при каждой сборке. При старте сервер дозаполняет ASN у старых сессий и отправляет на пересчёт свежие сессии из сетей облаков и хостингов.

`AGGREGATE_TTL=0` отключает удаление агрегатов. Сроки задаются в формате Go duration
(`168h`, а не `7d`). `RAW_EVENTS_TTL` — минимум `1h`, `STALE_SESSION_AFTER` —
минимум `1m`, `FINISH_GRACE` и `AGGREGATE_TTL` не могут быть отрицательными.
Рабочий цикл фоновых задач фиксирован: 5 секунд.

При старте `ADMIN_USERNAME`/`ADMIN_PASSWORD` создают или обновляют администратора.
Изменение пароля в окружении обновляет bcrypt-хеш и завершает его прежние сессии.
Bootstrap-настройки проекта применяются только при создании: существующий проект
с тем же публичным ключом сохраняет имя, origins и scoring. Меняйте их в панели.
Пустой ключ при прямом запуске бинарника генерирует новый проект при каждом старте;
Compose требует постоянный ключ. Секреты интеграций задавайте только на сервере.
