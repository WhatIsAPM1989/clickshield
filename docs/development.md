# Локальная разработка

[Документация](README.md) · [Правила изменений](../CONTRIBUTING.md)

## Инструменты

- Go 1.26+ и Make; используйте версию из `go.mod` или более новую совместимую.
- Node.js 22+ и npm для проверок/обновления frontend-бандлов. Сам SDK поддерживает Node.js 18+.
- PostgreSQL 17 для интеграционных тестов; Docker Compose v2 для контейнерного запуска.
- Python 3 нужен только для demo-сайта и проверки развёртывания.

```bash
npm ci --ignore-scripts
make help
make check
make build
```

`make check` не изменяет исходники: проверяет gofmt, `go vet`, race-тесты Go,
синтаксис всех JS/MJS в assets/scripts/sdk и тесты SDK/приватности replay.
`make fmt` форматирует Go. `go mod verify` проверяет загруженные зависимости.
`go mod tidy` выполняйте при изменении зависимостей и проверяйте diff, а не при каждом запуске.

## Тесты PostgreSQL

Используйте только отдельную БД: тесты создают/удаляют проекты и запускают очистку
данных. Рабочая БД и её копия с персональными данными для этого не подходят.
Без `CLICKSHIELD_TEST_DATABASE_URL` эти тесты пропускаются.

```bash
docker run --detach --name clickshield-test-db \
  -e POSTGRES_DB=clickshield_test \
  -e POSTGRES_USER=clickshield_test \
  -e POSTGRES_PASSWORD=test-only \
  -p 127.0.0.1:55432:5432 postgres:17-alpine
docker exec clickshield-test-db pg_isready -U clickshield_test -d clickshield_test
# Дождитесь сообщения accepting connections.
export CLICKSHIELD_TEST_DATABASE_URL='postgres://clickshield_test:test-only@localhost:55432/clickshield_test?sslmode=disable'
make test-integration
```

Пакеты выполняются последовательно (`-p 1`), поскольку используют общую тестовую
БД. Тесты сами применяют SQL-миграции. После работы удалите только тестовый контейнер:

```bash
docker rm -f clickshield-test-db
```

GitHub Actions поднимает PostgreSQL 17 и запускает `make check` с тестовым DSN;
поэтому проверки хранилища и HTTP API не пропускаются.

## Запуск бинарника

`make run` и `bin/clickshield` читают переменные процесса, а не `.env`.
Экспортируйте обязательные значения из [конфигурации](configuration.md), включая
`DATABASE_URL`, пароль администратора и криптографические ключи. Если запускаете
PostgreSQL отдельно, укажите его реальный адрес. В обычном Compose порт БД
на хост не опубликован.

Самый простой запуск всей системы — `make docker-up` после заполнения `.env`.
Assets и миграции встроены в Go-бинарник: после их изменения нужна пересборка.

## Frontend и генерируемые файлы

Обновление rrweb:

```bash
# Измените точные версии в package.json.
npm install --ignore-scripts
npm run vendor
make check
```

Проверьте запись/плеер в браузере и включите в изменение `package-lock.json`,
обновлённые JS/CSS и лицензионное уведомление. Для проверки текущих версий
достаточно `npm ci --ignore-scripts` и `npm run vendor`: diff assets должен быть пустым.

Каталог опубликованных IP-диапазонов ботов обновляется через сеть:

```bash
go generate ./internal/bots
```

Проверьте diff `internal/bots/ranges.go`. Не меняйте vendored и генерируемые
файлы вручную. Для UI-проверки используйте [demo-сайт](../examples/demo-site/index.html)
и [чек-лист](validation.md).
