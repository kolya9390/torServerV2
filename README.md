# TorrServerV2 — Self-hosted Torrent Streaming Backend

[![Build Status](https://github.com/kolya9390/torServerV2/actions/workflows/ci.yml/badge.svg)](https://github.com/kolya9390/torServerV2/actions)
[![License](https://img.shields.io/github/license/YouROK/TorrServer)](LICENSE)

> Надежный домашний torrent streaming backend для 1-2 heavy 4K streams, низкой нагрузки в release-режиме и удобного CLI-управления.

Текущий release channel: **beta**. Стабильная гарантия совместимости начинается только с `v1.0.0`. Правила версий и
публичный контракт описаны в [VERSIONING.md](VERSIONING.md).

Проектные документы: [Changelog](CHANGELOG.md) и [версионирование и release policy](VERSIONING.md).

---

## ℹ️ О проекте

TorrServerV2 основан на [YouROK/TorrServer](https://github.com/YouROK/TorrServer) и развивается как
предсказуемый self-hosted сервер для домашнего просмотра торрентов через HTTP/DLNA.

Главная цель проекта - стабильный streaming engine для домашней сети: запустили сервер на Mac, mini-PC,
NAS или домашнем Linux-сервере, подключили ТВ/плеер/клиент и смотрите фильмы без ручной возни с файлами.

**Рекомендуемые клиенты для просмотра:**

- **[TorrServe](https://github.com/YouROK/TorrServe)** — Android-клиент для управления сервером, поиска торрентов, добавления и выбора плеера.
- **[Lampa](https://github.com/yumata/lampa-source)** — приложение для Smart TV (WebOS, Tizen, Android TV). Подключается к TorrServer через плагин и позволяет искать и смотреть торренты прямо на телевизоре.

**Совместимость:**
- ✅ **[Lampa](https://github.com/lampa-app/LAMPA): Полная совместимость подтверждена.** Сервер успешно протестирован с приложением Lampa на Smart TV (WebOS, Tizen, Android TV).
- ✅ **Протестирован** с TorrServe, VLC, MPV
- ✅ **API совместим** с оригинальной версией YouROK/TorrServer
- ✅ **DLNA** работает с любыми DLNA-клиентами (TV, плееры)

---

## 📖 Что это?

**TorrServerV2** состоит из двух небольших программ с разными обязанностями. `torrserver` — долгоживущий daemon,
который запускает torrent engine, HTTP API и DLNA. `torrctl` — короткоживущий HTTP-клиент для локального или удалённого
управления daemon. Сервер скачивает torrent on-demand, кэширует нужные части и отдаёт медиапоток через HTTP/DLNA.

**Философия:**
- ✅ **Стабильность** — основной target: 1 heavy 4K stream; recommended target: 2 heavy 4K streams при подходящих ресурсах.
- ✅ **Низкий release overhead** — debug endpoints, pprof и тяжелые diagnostics выключены по умолчанию.
- ✅ **Предсказуемость** — streaming profiles, безопасный shutdown/drop и понятная диагностика проблем.
- ✅ **Управляемость** — отдельный `torrctl` для локального и удалённого управления без запуска torrent engine.

**Что умеет:**
- Стриминг торрентов через HTTP
- DLNA сервер для TV (Kodi, VLC, WebOS, Tizen)
- M3U плейлисты
- HTTP API для автоматизации
- CLI-клиент `torrctl` для управления сервером
- Управление пользователями (авторизация)

---

## 🎯 Целевые сценарии

| Сценарий | Ожидание |
|----------|----------|
| **Idle** | Почти нулевая CPU-нагрузка, без тяжелого фонового мониторинга |
| **1 heavy 4K stream** | Основной стабильный сценарий |
| **2 heavy 4K streams** | Целевой recommended сценарий при нормальной машине, сети и swarm |
| **3-4 heavy 4K streams** | Best effort, без продуктовой гарантии стабильности |

Важно: сервер может контролировать свой lifecycle, cache, HTTP streaming, release/debug policy и resource overhead.
Но torrent streaming зависит от swarm, seeders, trackers, WAN/LAN, клиента, Wi-Fi и bitrate файла. Поэтому
стабильность конкретного фильма всегда зависит не только от сервера.

### Честное performance-позиционирование

TorrServerV2 развивается как конкурентный домашний streaming engine, но не обещает бесконечную конкурентность любой
ценой. Продуктовая цель - надежный просмотр 1-2 heavy 4K фильмов на рекомендуемом железе без постоянной debug-нагрузки.

Текущий measured verdict:

| Область | Статус |
|---------|--------|
| **Idle / release overhead** | Сильная сторона TorrServerV2: debug endpoints, pprof и тяжелые stream diagnostics выключены при `debug.enabled=false`. |
| **1 heavy 4K stream** | Основной поддерживаемый сценарий. Для экстремальных remux файлов успех зависит от sustained bitrate, swarm и LAN/client path. |
| **2 unique heavy 4K streams** | Целевой режим на recommended hardware. Третий уникальный heavy stream может быть ограничен, поставлен в очередь или отклонен, чтобы не ломать первые два. |
| **2 клиента на одном фильме** | Должно переиспользовать один torrent/cache workload и не считаться как два разных heavy фильма. |
| **Оригинальный TorrServer** | Может быть лучше на отдельных swarm/client/startup сценариях из-за зрелых исторических defaults. TorrServerV2 сильнее там, где важны явные limits, debug-off overhead, privacy-safe diagnostics и предсказуемый lifecycle. |

Если фильм подвисает, это не всегда означает баг в Go-коде. Для heavy remux нужно проверять три вещи отдельно:
delivery throughput сервера, качество torrent swarm и путь клиент/LAN/декодер. Для этого debug mode включается
временно, а не работает как постоянный мониторинг.

---

## 🚀 Быстрый старт

### Platform bundle (Linux / macOS)

Каждый platform bundle содержит `torrserver`, `torrctl` и release-safe `config.example.yml`. Выберите точную
опубликованную версию и платформу на [странице релизов](https://github.com/kolya9390/torServerV2/releases), затем
скачайте bundle и общий checksum manifest:

```bash
version="${TORRSERVER_VERSION:?set published version without v prefix}"
platform="darwin-arm64"  # linux-amd64, linux-arm64, darwin-amd64 или darwin-arm64
bundle="TorrServerV2-v${version}-${platform}.tar.gz"
base="https://github.com/kolya9390/torServerV2/releases/download/v${version}"

curl -fLO "${base}/${bundle}"
curl -fLO "${base}/torrserver-${version}-SHA256SUMS"
grep "  ${bundle}$" "torrserver-${version}-SHA256SUMS" | shasum -a 256 --check -
tar -xzf "${bundle}"
cd "TorrServerV2-v${version}-${platform}"
```

На Linux вместо `shasum -a 256 --check -` можно использовать `sha256sum --check -`. Для Windows скачайте `.zip`,
проверьте SHA-256 через PowerShell и используйте `torrserver.exe`/`torrctl.exe`; полный пример приведён в
[VERSIONING.md](VERSIONING.md#установка-из-platform-bundle).

Скопируйте пример конфигурации, создайте runtime-директории и запустите daemon в foreground:

```bash
cp config.example.yml config.yml
mkdir -p state logs
TS_CONFIG="$PWD/config.yml" ./torrserver serve --path "$PWD/state" --logpath "$PWD/logs"
```

Оставьте daemon в первом терминале. Во втором терминале выполните полный management/stream URL workflow:

```bash
./torrctl status
./torrctl torrents add "magnet:?xt=urn:btih:<INFO_HASH>" --title "Movie" --save
./torrctl torrents list
./torrctl url 1 --list
./torrctl url 1 --file 1
./torrctl shutdown
```

Локальный `.torrent` можно загрузить вместо magnet: `./torrctl torrents add ./movie.torrent --save`. Команда
`url` ограниченно ждёт metadata; для медленного swarm увеличьте только клиентский timeout, например
`./torrctl --timeout 45s url 1 --list`.

### Docker

Docker image содержит и запускает только daemon:

```bash
cp config.example.yml config.yml

docker run -d \
  --name torrserver \
  -p 8090:8090 \
  -p 9080:9080 \
  -e TS_CONFIG=/opt/ts/config/config.yml \
  -v torrserver-data:/opt/ts \
  -v "$PWD/config.yml:/opt/ts/config/config.yml:ro" \
  ghcr.io/kolya9390/torserverv2:1.0.0-beta.3 \
  --path /opt/ts/config --logpath /opt/ts/log
```

`torrctl` не нужен внутри container. Управляйте daemon с host-бинарника:

```bash
./torrctl --server http://127.0.0.1:8090 status
```

Lampa, TorrServe и обычные HTTP/DLNA-клиенты подключаются напрямую к daemon и не зависят от `torrctl`. Для
воспроизводимой установки используйте точный image tag или digest. `latest` предназначен только для stable-релиза.
Чтобы сохранять `.torrent` и disk cache отдельно от runtime state, задайте в YAML путь
`cache.torrents_save_path: /opt/ts/torrents` и используйте отдельный volume; пустое значение сохраняет memory-only
поведение.

---

## 💻 Daemon и CLI

`torrserver` и `torrctl` намеренно разделены:

| Программа | Lifecycle | Ответственность |
|-----------|-----------|-----------------|
| `torrserver` | Долгоживущий foreground process | Torrent engine, cache, HTTP API, streaming, DLNA и graceful shutdown |
| `torrctl` | Один короткий process на команду | HTTP management client; не запускает и не supervises OS process daemon |

Канонический запуск — `torrserver serve`. `torrctl` по умолчанию обращается к `http://127.0.0.1:8090`, поэтому для
локального сервера достаточно `torrctl status`. Для удалённого сервера используйте глобальный `--server` или context.

### Основные команды

```bash
# Локальная build-информация; сеть не используется
./torrserver --version
./torrctl --version
./torrctl version --output json

# Torrent workflow
./torrctl status
./torrctl torrents list
./torrctl torrents add "magnet:?xt=urn:btih:<INFO_HASH>" --save
./torrctl torrents add ./movie.torrent --title "Movie" --save
./torrctl torrents get 1
./torrctl url 1 --list
./torrctl url 1 --file 3

# Полученную ссылку можно передать внешнему плееру
mpv "$(./torrctl url 1 --file 3)"

# Settings и lifecycle
./torrctl settings get
./torrctl settings set CacheSize 128MB
./torrctl shutdown
```

Идентификатор торрента может быть индексом из `torrents list`, названием или hash. Для `url` без `--file` выбирается
самый большой файл. `--file` принимает ID или часть имени. У сохранённого, но неактивного торрента metadata может ещё
не быть в базе: повторно добавьте hash с `--save` и дождитесь metadata либо передайте уже известный file ID.

### Удалённые серверы и contexts

Одноразовый вызов не изменяет конфигурацию `torrctl`:

```bash
./torrctl --server http://192.168.1.50:8090 status
```

Для повторного использования создайте именованный context:

```bash
./torrctl context add --name home --server http://192.168.1.50:8090
./torrctl --context home torrents list
./torrctl context use --name home
./torrctl status
./torrctl context use --name local
```

Локальный `.torrent` читается на машине с `torrctl` и загружается на выбранный remote server через multipart API.
Приоритет настроек клиента: явные флаги, environment, выбранный context, затем `local` default.

### Конфигурация daemon и клиента

Это два независимых конфигурационных контура:

- `torrserver` читает YAML из `TS_CONFIG` либо ищет `config.yml` в документированных стандартных путях. `--path`,
  `--logpath` и другие daemon flags задают runtime state/lifecycle и не являются настройками `torrctl`.
- `torrctl` не читает server YAML. Contexts хранят URL и необязательные credentials в пользовательском
  `tsctl/config.json` с правами `0600`; путь можно переопределить через `TSCTL_CONFIG`.
- `TSCTL_CONTEXT`, `TS_USER`, `TS_PASSWORD` и `TS_SHUTDOWN_TOKEN` позволяют задавать client state без записи секрета
  в context. Для automation это предпочтительнее command-line flags, видимых в process list и shell history.

### JSON-контракт

Для автоматизации используйте глобальный `--output json`. Успешная команда печатает в `stdout` один JSON-документ:

```json
{
  "ok": true,
  "data": {
    "url": "http://127.0.0.1:8090/streams/play?index=1&link=...",
    "torrent_hash": "...",
    "file_id": 1
  }
}
```

При ошибке `stdout` остаётся пустым, exit code ненулевой, а `stderr` содержит один JSON-документ:

```json
{
  "ok": false,
  "error": {
    "code": "validation_error",
    "message": "invalid torrent link",
    "status": 422,
    "field": "link",
    "request_id": "request-123"
  }
}
```

Password, token, URL credentials и произвольное HTTP error body не выводятся. Progress, prompts и диагностика идут в
`stderr`, поэтому успешный JSON в `stdout` можно передавать в `jq`.

### TLS, authentication и shutdown token

`--server` и contexts принимают абсолютные `http://` или `https://` URL. Reverse-proxy path сохраняется, redirects не
проходятся автоматически. `--insecure` отключает проверку TLS certificate и допустим только для контролируемой сети с
self-signed certificate.

Для HTTP Basic Auth включите `server.http_auth: true` в YAML или запустите daemon с `--httpauth`, затем создайте
пользователя через `torrctl auth add admin`. Если указан `--user`, но password отсутствует, интерактивный `torrctl`
безопасно запросит его. Для automation используйте environment:

```bash
export TS_USER=admin
export TS_PASSWORD='<PASSWORD>'
./torrctl torrents list
```

Публичный shutdown требует `server.shutdown_mode: public` в daemon YAML и token не короче 16 символов:

```bash
export TS_SHUTDOWN_TOKEN="$(./torrctl config shutdown-token generate --yes)"
./torrctl config shutdown-token status
./torrctl shutdown --mode public
```

Не передавайте password/token через `--pass` или `--token` в обычной работе: значения могут попасть в process list и
shell history. Не публикуйте API в интернет без TLS, authentication и ограничений reverse proxy. В release config
оставляйте `debug.enabled: false`, иначе становятся доступны чувствительные profiling endpoints.

### Миграция с прежнего mixed binary

В prerelease-версиях management и daemon находились в одном `torrserver`. Новый контракт явный:

| Прежняя команда | Новая команда |
|-----------------|---------------|
| `torrserver` или `torrserver --port 8090` | `torrserver serve` или `torrserver serve --port 8090` |
| `torrserver status` | `torrctl status` |
| `torrserver torrents list` | `torrctl torrents list` |
| `torrserver torrents add ...` | `torrctl torrents add ...` |
| `torrserver url 1` | `torrctl url 1` |
| `torrserver settings get` | `torrctl settings get` |
| `torrserver shutdown` | `torrctl shutdown` |

`torrserver` временно возвращает ограниченную подсказку для старых management-команд, но не выполняет их внутри
daemon. Это prerelease migration aid, а не обещание бессрочной совместимости alias.

---

## 📘 API Документация (Swagger)

После запуска сервера документация API доступна по адресу:
👉 [http://localhost:8090/swagger/index.html](http://localhost:8090/swagger/index.html)

В Swagger UI вы можете:
- Просмотреть все доступные эндпоинты (`/torrents`, `/settings`, `/stream` и др.)
- Изучить форматы запросов и ответов
- Тестировать API прямо из браузера (кнопка **"Try it out"**)

---

## 🛠️ Сборка

```bash
make build          # torrserver и torrctl
make test           # Тесты
make generate-mocks # Моки через mockgen
make swagger        # Обновить документацию API
docker build -t torrserver .  # Docker
```

**Требования:** Go 1.26+

---

## ⚙️ Конфигурация

### Флаги запуска

| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `--port` | `8090` | Порт API |
| `--ip` | все интерфейсы | Адрес привязки HTTP server |
| `--path` | текущая директория | Runtime data/state directory |
| `--logpath` | `./` | Путь для логов |
| `--httpauth` | `false` | Включить защиту паролем |
| `--shutdownmode` | `local` | Режим shutdown: `local` или `public` |

Полный список daemon flags доступен через `torrserver serve --help`. Путь к YAML задаётся отдельно через
`TS_CONFIG`; каталог torrent/disk cache задаётся полями `cache.torrents_save_path` и `disk_cache` в YAML.

### Переменные окружения

| Переменная | Владелец | Описание |
|------------|----------|----------|
| `TS_CONFIG` | `torrserver` | Точный путь к daemon YAML |
| `TSCTL_CONFIG` | `torrctl` | Точный путь к client context JSON |
| `TSCTL_CONTEXT` | `torrctl` | Context для текущего вызова |
| `TS_USER` | `torrctl` | HTTP Basic Auth user |
| `TS_PASSWORD` | `torrctl` | HTTP Basic Auth password |
| `TS_SHUTDOWN_TOKEN` | daemon и client | Token для public shutdown |

### Streaming profiles

Основной release default остается compatibility-first:

```yaml
streaming:
  core_profile: "custom"
```

Для тяжелого домашнего просмотра можно использовать профиль:

```yaml
streaming:
  core_profile: "tcp-only-balanced"
```

`tcp-only-balanced` снижает часть сетевого overhead и является recommended Home 4K profile для 1-2 heavy streams
на подходящей домашней машине, но не включен по умолчанию, чтобы сохранить broad compatibility. Если конкретному swarm
нужен более широкий transport reach, вернитесь к `custom`.

`low-cpu` предназначен для CPU-constrained устройств и не является общей рекомендацией для тяжелых 4K streams.

### Network peer discovery

BEP-14 Local Peer Discovery добавлен как opt-in режим для измеряемых home/LAN экспериментов:

```yaml
network:
  enable_lpd: false
  lpd_ipv6: false
```

LPD может быстрее находить локальных BitTorrent-пиров в домашней сети и приближает peer discovery к TorrServer.
Runtime A/B показал рост discovered/queued peers, но не доказал улучшение playback health для heavy Home 4K сценария.
Поэтому LPD выключен по умолчанию, чтобы не добавлять multicast/noise и privacy surface без доказанной пользы.
IPv6 LPD включайте только если IPv6 в локальной сети реально используется и `enable_ipv6: true`.

### Production / public deployment

- Держите `debug.enabled: false` в release и internet-exposed deployments. Полный debug mode публикует
  `/debug/pprof/*`, `/debug/vars`, heap и goroutine diagnostics, поэтому он предназначен только для доверенного
  локального profiling.
- Для локальной диагностики временно включите `debug.enabled: true`, соберите profile/snapshot и верните значение
  обратно в `false`. Используйте `debug.service_only: true`, когда нужны debug logs TorrServerV2 без HTTP debug
  endpoints и torrent library debug noise.
- CORS allow-all остается compatibility mode для домашних media clients и Smart TV apps. Для VPS, reverse proxy или
  internet-exposed deployments задайте `TS_CORS_ALLOW_ORIGINS` как comma-separated allowlist, например
  `https://example.com,https://app.example.com`.
- `TS_CORS_ALLOW_PRIVATE_NETWORK=1` стоит включать только когда браузерам в доверенной локальной сети нужна поддержка
  Private Network Access preflight.

---

## 📊 Ресурсы

Ресурсы зависят от bitrate, количества активных streams, качества swarm, cache mode, Wi-Fi/LAN и клиента.
Ниже - практический target для домашнего использования, а не синтетический максимум.

| Профиль машины | Ресурсы | Ожидание |
|----------------|---------|----------|
| **Minimum supported** | 4 GB RAM total, желательно 2 GB свободно до запуска, 4-core CPU | 1 heavy stream как основная цель; 2 heavy streams best effort |
| **Recommended** | 8 GB RAM total, 3-4 GB свободно, Apple Silicon M1+ или x86_64 4+ cores, SSD | Целевой режим для стабильных 2 heavy 4K streams |
| **Comfort / headroom** | 16 GB RAM total, 6 GB свободно, современный CPU, SSD | 2 heavy streams стабильно; 3-4 streams возможны, но остаются best effort |

Ориентиры для recommended режима:

- TorrServer memory budget: обычно 300-800 MB, с пиками до 1-1.5 GB в тяжелых сценариях.
- WAN: стабильные фактические 250 Mbps+ download для двух heavy 4K streams.
- LAN: стабильные 300-500 Mbps, желательно Ethernet или хороший Wi-Fi 6/6E.
- Storage: SSD предпочтителен, особенно если используется disk cache.

Для нескольких heavy 4K streams сеть может стать главным bottleneck раньше, чем CPU. Один 4K файл может потреблять
20-100+ Mbps, а два тяжелых remux-потока требуют заметного запаса на пики, retransmits и поведение клиента.

---

## 🔎 Диагностика и debug mode

TorrServerV2 не должен постоянно собирать тяжелые runtime diagnostics в release-режиме.

По умолчанию:

- `debug.enabled=false`;
- `/debug/pprof/*`, `/debug/vars`, heap и goroutine diagnostics не публикуются;
- тяжелые stream diagnostics и runtime metric updater не работают;
- debug-only torrent knobs игнорируются.

Для локального анализа проблемы временно включите:

```yaml
debug:
  enabled: true
```

После сбора профилей и snapshot верните `debug.enabled: false`.

---

## 📄 Лицензия

GPL 3.0 — см. [LICENSE](LICENSE)
