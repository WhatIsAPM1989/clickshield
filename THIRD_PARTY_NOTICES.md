# Сторонние компоненты

ClickShield распространяется по [MIT](LICENSE). Сторонние материалы сохраняют
свои лицензии и уведомления.

| Компонент | Использование | Лицензия / уведомление |
|---|---|---|
| minio-go/v7 | S3-клиент для replay | Apache 2.0, лицензия в модуле `github.com/minio/minio-go/v7`; версия в `go.mod` |
| rrweb record/replay 2.1.7 | Встроенные JS/CSS для записи и плеера | MIT, [уведомление](internal/httpapi/assets/rrweb-LICENSE.txt); версии закреплены в `package-lock.json` |
| MaxMind GeoIP2 City test database | Только тесты GeoIP | [Происхождение](internal/geo/testdata/README.md), [Apache 2.0](internal/geo/testdata/LICENSE-APACHE), [MIT](internal/geo/testdata/LICENSE-MIT) |
| iptoasn IP→ASN database | Docker-сборка загружает базу сетей | Public domain по условиям поставщика; URL задаётся `ASN_DATABASE_URL` в Dockerfile |
| GeoLite2 City (MaxMind) | Docker-сборка загружает базу геолокации через неофициальное зеркало P3TERX/GeoLite.mmdb | © MaxMind, [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/) и GeoLite2 EULA; URL задаётся `GEOIP_CITY_DATABASE_URL` в Dockerfile |

Go-зависимости и транзитивные версии перечислены в `go.mod`/`go.sum`.
Рабочие GeoIP2/GeoLite2 базы не входят в репозиторий; получайте и используйте
их на условиях поставщика. Обновляя встроенные зависимости, сохраняйте
соответствующие лицензионные уведомления.
