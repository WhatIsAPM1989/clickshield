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
| `ADMIN_PASSWORD` | пароль bootstrap, минимум 12 символов | обязательно для bootstrap |
| `SESSION_SECRET` | HMAC-ключ серверных admin-сессий, минимум 32 байта | обязательно |
| `ENCRYPTION_KEY` | base64 от 32 байт для AES-GCM | обязательно |
| `COOKIE_SECURE` | cookie только по HTTPS | `false` в dev |
| `TRUST_PROXY_HEADERS` | доверять `CF-Connecting-IP` / `X-Forwarded-For` | `false` |
| `BOOTSTRAP_PROJECT_NAME` | имя первого проекта | `Demo site` |
| `BOOTSTRAP_PROJECT_KEY` | постоянный публичный ключ первого проекта | обязательно для bootstrap |
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

`TRUST_PROXY_HEADERS=true` допустим только за доверенным proxy/CDN, который очищает входящие заголовки. ASN берётся из `X-GeoIP-ASN`/`CF-ASN`, организация — из `X-GeoIP-Organization`. Если proxy их не передаёт, ASN и организация определяются по базе [iptoasn.com](https://iptoasn.com) (public domain). Docker-образ скачивает её при каждой сборке. Дозаполнение старых сессий выполняется явно через `clickshield backfill networks`; `serve` только загружает файл справочника.

`AGGREGATE_TTL=0` отключает удаление агрегатов. Сроки задаются в формате Go duration
(`168h`, а не `7d`). `RAW_EVENTS_TTL` — минимум `1h`, `STALE_SESSION_AFTER` —
минимум `1m`, `FINISH_GRACE` и `AGGREGATE_TTL` не могут быть отрицательными.
Scoring запускается worker каждые 5 секунд, доставка — каждые 2 секунды,
проекции — каждую секунду, порционная очистка — каждую минуту.

Команда `bootstrap` создаёт или обновляет администратора. Изменение пароля при
повторном bootstrap обновляет bcrypt-хеш и завершает прежние admin-сессии.
`serve` и `worker` администратора не изменяют. Bootstrap создаёт проект и
интеграции только при отсутствии; последующие изменения через панель сохраняются.

`migrate` и `backfill` требуют только `DATABASE_URL` (для replay также S3).
`serve`/`worker` требуют `SESSION_SECRET` и `ENCRYPTION_KEY`; секреты администратора
нужны только `bootstrap`. `rescore` и `reproduce` требуют `SESSION_SECRET`.

## Replay в S3

| Переменная | Назначение |
|---|---|
| `REPLAY_S3_ENDPOINT` | origin S3 без пути, например `https://s3.example.com` |
| `REPLAY_S3_BUCKET` | закрытый bucket для записей |
| `REPLAY_S3_ACCESS_KEY` | серверный ключ доступа |
| `REPLAY_S3_SECRET_KEY` | серверный секрет |
| `REPLAY_S3_REGION` | регион, по умолчанию `us-east-1` |

Если endpoint задан, bucket и оба ключа обязательны. Без S3 нельзя включить новую
запись; tracker получает `replay_enabled=false`. Bootstrap создаёт bucket, если
его ещё нет. Для заранее созданного bucket это разрешение не требуется.
Используйте адрес, доступный из контейнеров; `localhost` внутри app означает сам
контейнер. Браузер обращается только к ClickShield, доступ к S3 ему не выдаётся.
