# ClickShield Analytics SDK

JS/TypeScript-клиент чтения аналитики для Node.js 18+ или серверной среды с `fetch`.
Нужен приватный API-токен с правом `analytics:read`. Выпустите его в панели,
раздел «API и MCP»; не включайте токен в браузерный bundle. Публичный ключ трекера для SDK не подходит.

## Подключение

Из checkout репозитория импортируйте `sdk/index.js`. Чтобы использовать локальный
пакет в другом Node-проекте, выполните `npm install /path/to/clickshield/sdk`,
затем импортируйте `@clickshield/analytics`.

```js
import { ClickShieldAnalytics } from './sdk/index.js';

const analytics = new ClickShieldAnalytics({
  baseURL: 'https://clickshield.example.com',
  apiKey: process.env.CLICKSHIELD_ANALYTICS_KEY,
});

const filters = {
  from: '2026-10-01T00:00:00+03:00',
  to: '2026-11-01T00:00:00+03:00',
  classification: 'NO_BOTS',
  timezone: 'Europe/Moscow',
};
const sources = await analytics.report('sources', filters);
const timeline = await analytics.timeline('sources', {
  ...filters,
  interval: 'day',
});
const funnels = await analytics.behavior('funnels', filters);
```

`from` включительно, `to` исключительно. Можно передавать `Date` вместо строк.
`project_id` можно опустить для токена одного проекта; другой проект отклоняется.
`NO_BOTS` исключает `BOT` и `KNOWN_BOT`.

| Метод | Результат |
|---|---|
| `report(name, filters, options)` | Иерархия строк, итоги и часовой пояс отчёта |
| `timeline(name, filters, options)` | Временные интервалы, итоги и серии |
| `behavior(kind, filters, options)` | Данные events, properties, goals, funnels, journey, heatmap или retention |
| `definitions(projectID, options)` | Список определений целей/воронок |
| `tools(projectID, kind, filters, options)` | Ссылки, пиксели или отметки (`links`, `pixels`, `annotations`) |

Последний аргумент `{signal}` позволяет отменить запрос через `AbortController`.
Неуспешный HTTP-ответ вызывает `Error` с полем `status`; истёкший или отозванный
ключ вернёт 401. Перенаправления запрещены. Типы находятся в `index.d.ts`.

## Бизнес-события в браузере

Для записи событий установите публичный tracker; SDK их не отправляет.
После инициализации вызовите:

```js
window.ClickShield.track('call', {placement: 'header'});
```

Либо добавьте на кнопку `data-cs-event="call" data-cs-prop-placement="header"`.
Параметры поддерживают строки, конечные числа, boolean и null; до 32 ключей,
8 KiB суммарно. Не передавайте персональные данные. Подробнее — в
[руководстве продуктовой аналитики](../docs/product-analytics.md).

## Проверка

Из корня репозитория: `node --test sdk/index.test.mjs` или `make test-js`.
[HTTP API](../docs/openapi.yaml) · [Документация](../docs/README.md).
