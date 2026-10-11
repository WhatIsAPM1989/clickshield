# Проверка ClickShield перед production

## Автоматические проверки

```bash
npm ci --ignore-scripts
make check
make test-integration  # требует CLICKSHIELD_TEST_DATABASE_URL
# После заполнения .env:
docker compose config --quiet
```

`make check` проверяет форматирование, `go vet`, Go race-тесты, синтаксис JS и
тесты SDK/приватности replay. Без `CLICKSHIELD_TEST_DATABASE_URL` Go пропускает
тесты с PostgreSQL. [Запуск отдельной тестовой БД](development.md). CI запускает
их обязательно. Этот чек-лист не является записью о текущем деплое.


## Smoke test

1. Создать `.env`, заменить все dev-секреты и запустить `docker compose up --build`.
2. Проверить `GET /healthz` и вход в панель.
3. Добавить origin тестового сайта в проект и подключить сниппет tracker.
4. Убедиться, что один batch с одинаковым `batch_id` не создаёт дубликаты в `raw_events`.
5. Перейти между двумя страницами быстрее `FINISH_GRACE`: должна остаться одна активная сессия с увеличенным `page_count`.
6. После затишья проверить `session_features`, classification и причины.
7. Поставить ручную метку `BOT`, затем дождаться повторного automatic scoring: ручная метка должна остаться текущей.
8. В `MONITORING` решение tracker должно быть `MONITOR`; в `BLOCKING` для ранее подтверждённого бота — `BLOCK`.
9. Проверить, что origin вне allowlist получает `403`, а oversized/unknown JSON — `400/422`.
10. Проверить очистку raw events на тестовом коротком TTL и сохранность агрегатов до `AGGREGATE_TTL`.

## Яндекс Метрика

- Создать Measurement Protocol token и цели `antifraud_bot`, `antifraud_human`.
- Убедиться, что tracker получил числовой Yandex ClientID.
- Включить интеграцию и разметить тестовую сессию.
- Проверить статус outbox: `PENDING/RETRY` должен перейти в `SENT` либо дать безопасную ошибку без токена.
- Проверить достижение цели в Метрике для нужного ClientID.

## Google Analytics 4

- Создать Measurement Protocol API secret для нужного web stream.
- Убедиться, что tracker получил GA `client_id` вида `123.456`.
- Проверить payload через GA4 validation endpoint/DebugView до боевого включения.
- Учесть, что Measurement Protocol может вернуть HTTP 2xx даже при некоторых семантических ошибках payload; статус `SENT` означает успешную HTTP-доставку, а не гарантированное появление события в отчётах.
- Создать аудиторию по `antifraud_bot` только после проверки false-positive rate.

## Нагрузка и эксплуатация

- Прогнать реальные батчи через reverse proxy с теми же лимитами body/timeouts.
- При `TRUST_PROXY_HEADERS=true` убедиться, что proxy очищает входящие `X-Forwarded-For`, `X-GeoIP-*`, `CF-*` заголовки.
- Проверить размер `raw_events`, планы запросов dashboard и скорость cleanup на объёме, близком к ожидаемому.
- Для нескольких реплик вынести rate limit/velocity cache в Redis либо внешний gateway; PostgreSQL outbox уже поддерживает конкурентных воркеров через `SKIP LOCKED`.
- Настроить резервные копии PostgreSQL, HTTPS, ротацию секретов и алерты на рост `RETRY/FAILED` в outbox.

## Аналитика и replay

- Включить запись сессий в проекте, 100%, 7 дней.
- Открыть сайт с UTM и click ID, перейти на другую страницу без меток:
  источник, кампания и страница входа сохраняются, история содержит оба URL.
- После истечения сессии новый визит без меток не наследует click ID.
- Проверить обычный внешний referrer, прямой заход и внутренний переход.
- Проверить pushState/replaceState/popstate, отсутствие дубля при ручном pageview.
- Ввести уникальное значение в форму и создать блок data-cs-private:
  уникальные строки должны отсутствовать в JSON из `/v1/sessions/{id}/replay`.
- Проверить источник → список визитов → карточку → плеер; две страницы,
  play/pause, seek, speed. Внутренний iframe rrweb имеет sandbox без allow-scripts.
- Проверить сообщения для отсутствующей/неполной записи, выключенной записи,
  сетевой ошибки, просроченной записи; удаление из карточки.
- Проверить источник/канал/период/кампанию и CSV, пустой результат за другой период.
- Reporting API: отсутствие OAuth, неверный OAuth, сохранение токена без выдачи
  его клиенту, пагинацию и четыре отчёта. Рабочий счётчик требует metrika:read.
- Перед production-обновлением сделать backup; после — healthz, миграции,
  отсутствие ошибок в логах, реальный визит и его replay в рабочей панели.
