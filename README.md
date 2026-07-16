# TorrServerV2 — Self-hosted Torrent Streaming Backend

[![Build Status](https://github.com/kolya9390/torServerV2/actions/workflows/ci.yml/badge.svg)](https://github.com/kolya9390/torServerV2/actions)
[![License](https://img.shields.io/github/license/YouROK/TorrServer)](LICENSE)

> Надежный домашний torrent streaming backend для 1-2 heavy 4K streams, низкой нагрузки в release-режиме и удобного CLI-управления.

Текущий release channel: **beta**. Первый публичный кандидат планируется как `v1.0.0-beta.1`; стабильная гарантия
совместимости начинается только с `v1.0.0`. Правила версий и публичный контракт описаны в [VERSIONING.md](VERSIONING.md).

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

**TorrServerV2** — один бинарник, который содержит сервер и CLI для управления. Сервер скачивает torrent
on-demand, кэширует нужные части и отдает медиапоток клиентам через HTTP/DLNA.

**Философия:**
- ✅ **Стабильность** — основной target: 1 heavy 4K stream; recommended target: 2 heavy 4K streams при подходящих ресурсах.
- ✅ **Низкий release overhead** — debug endpoints, pprof и тяжелые diagnostics выключены по умолчанию.
- ✅ **Предсказуемость** — streaming profiles, безопасный shutdown/drop и понятная диагностика проблем.
- ✅ **Управляемость** — встроенный CLI для локального и удаленного управления сервером.

**Что умеет:**
- Стриминг торрентов через HTTP
- DLNA сервер для TV (Kodi, VLC, WebOS, Tizen)
- M3U плейлисты
- HTTP API для автоматизации
- CLI для управления сервером
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

### 1. Бинарный файл (Linux / macOS / Windows)

Скачайте файл с [страницы релизов](https://github.com/kolya9390/torServerV2/releases):

```bash
# Запуск сервера
./torrserver

# С настройками
./torrserver --port 8090 --path ./config --torrentsdir ./torrents
```

*(Для Windows используйте `torrserver.exe`)*

### 2. Docker

```bash
docker run -d \
  --name torrserver \
  -p 8090:8090 \
  -p 9080:9080 \
  -v ./config:/opt/ts/config \
  -v ./torrents:/opt/ts/torrents \
  ghcr.io/kolya9390/torserverv2:1.0.0-beta.1
```

Для воспроизводимой установки используйте точный тег опубликованной версии или digest. `latest` появится только у
stable-релиза и никогда не указывает на alpha, beta или RC. Политика каналов и rollback описаны в
[VERSIONING.md](VERSIONING.md).

### 3. Docker Compose

```bash
docker compose -f docker-compose.yml up -d
```

---

## 💻 CLI (встроен в сервер)

Один бинарник — два режима. Без аргументов запускает сервер, с аргументами работает как CLI:

```bash
# Запуск сервера (без аргументов)
./torrserver

# CLI команды (с аргументами)
./torrserver status
./torrserver torrents list
```

### JSON-контракт CLI

Для автоматизации добавьте глобальный флаг `--output json`. Он поддерживается командами `context`, `status`,
`torrents`, `url`, `settings`, `auth` и `shutdown`. Успешная команда печатает в `stdout` ровно один JSON-документ:

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

При ошибке `stdout` остаётся пустым, процесс возвращает ненулевой exit code, а `stderr` содержит один документ:

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

`status`, `field` и `request_id` присутствуют только когда их вернул API. Password, token, URL credentials и
произвольное тело HTTP-ошибки не выводятся; error message очищается и ограничивается по длине. Progress, prompts и
диагностика направляются в `stderr`, поэтому JSON в `stdout` можно безопасно передавать в `jq`.

### Адрес сервера и TLS

`--server` и сохраненные contexts принимают только `http://` или `https://` URL с host. Путь reverse proxy
сохраняется: для `--server https://home.example/torrserver` API-команды обращаются к
`https://home.example/torrserver/...`. CLI не следует HTTP redirects автоматически: указывайте конечный URL сервера,
чтобы credentials и request body не были перенаправлены на неожиданный адрес. `--insecure` отключает проверку TLS
certificate только по явному запросу и предназначен для контролируемых self-signed окружений.

CLI contract suite запускается отдельно командой `make test-cli` и входит в обычный `make test` и CI. Baseline на
2026-07-15: `64.9%` statements для `server/cmd/cli`; это индикатор регрессии покрытия, а не самостоятельная метрика
качества. Gate проверяет routing, contexts, uploads, URL selection, settings, auth, shutdown, cancellation, bounded
HTTP responses и machine-readable errors под race detector.

### Основные команды:

**Торренты:**
```bash
# Список торрентов (обратите внимание на колонку # — это индекс торрента)
./torrserver torrents list

# Добавить magnet-ссылку и сохранить торрент в базе
./torrserver torrents add "magnet:?xt=urn:btih:..." --title "Movie" --save

# Загрузить локальный .torrent-файл на сервер и сохранить его в базе
./torrserver torrents add ./movie.torrent --save

# Явная форма того же file upload
./torrserver torrents add --file ./movie.torrent --save

# Получить детали торрента (по индексу из списка, названию или хэшу)
./torrserver torrents get 1
./torrserver torrents get "Beef"
./torrserver torrents get ef9c7cd53234...

# Удалить торрент
./torrserver torrents rem 1
./torrserver torrents rem "Beef"

# Выгрузить из памяти (без удаления из БД)
./torrserver torrents drop "Beef"

# Удалить все торренты
./torrserver torrents wipe
```

**Стриминг (ссылки):**
```bash
# 1. Посмотрите список торрентов, чтобы узнать индекс
./torrserver torrents list
# Вывод:
# #  HASH          STATE  PEERS  DOWN  UP  TITLE
# 1  ef9c7cd5...   ...    ...    ...   ... Грызня (Beef) Сезон 1

# 2. Получите ссылку на стрим (используя индекс, имя или хэш торрента)
# Пример с индексом (цифра 1 из колонки # выше)
./torrserver url 1
# Вывод: http://127.0.0.1:8090/streams/play?link=ef9c7cd5...&index=1

# Пример с названием
./torrserver url "Beef"

# 3. Выбор конкретного файла внутри торрента

# Показать список файлов в торренте #1
./torrserver url 1 --list
# Вывод:
# ID  SIZE   NAME
# 1   2.3GB  Грызня - Beef S01 E01 ...
# 2   2.0GB  Грызня - Beef S01 E02 ...

# Получить ссылку на файл по ID (цифра из колонки ID в списке файлов)
./torrserver url 1 --file 3

# Получить ссылку на файл по части названия (удобно для выбора серии)
./torrserver url 1 --file "E05"
./torrserver url "Beef" --file "S01 E10"

# 4. Открыть в плеере
mpv "$(./torrserver url 1)"
vlc "$(./torrserver url "Beef")"
```

Команда `url` ограниченно ждёт, пока torrent engine получит metadata и список файлов. Если у раздачи нет доступных
пиров, команда завершится по `--timeout` с понятной ошибкой; увеличьте ожидание, например:

```bash
./torrserver --timeout 45s url 1
```

У старого сохранённого, но неактивного торрента список файлов может отсутствовать в базе. Если ID файла уже известен,
ссылку можно получить без активации движка: `./torrserver url 1 --file 1`. Иначе повторно добавьте его hash с `--save`,
дождитесь metadata и вызовите `url` ещё раз.

**Управление пользователями:**
```bash
# Показать список пользователей
./torrserver auth list

# Добавить нового пользователя (запросит пароль интерактивно)
./torrserver auth add admin

# Добавить пользователя с указанием пароля (для скриптов)
./torrserver auth add admin --password MySecretPass123

# Удалить пользователя
./torrserver auth remove admin
```

**Настройки:**
```bash
# Показать все настройки
./torrserver settings get

# Получить конкретную настройку
./torrserver settings get CacheSize

# Изменить настройку (поддержка суффиксов MB, GB и т.д.)
./torrserver settings set CacheSize 128MB
./torrserver settings set ConnectionsLimit 50

# Сбросить настройки
./torrserver settings def
```

**Управление сервером:**
```bash
# Краткая версия локального бинарника (без подключения к серверу)
./torrserver --version

# Полная build-информация локального бинарника
./torrserver version
./torrserver version --output json

# Статус и API version удалённого/запущенного сервера
./torrserver status

# Безопасная остановка сервера
./torrserver shutdown

# Остановка удалённого сервера (с токеном)
./torrserver shutdown --mode public --token my_secret_token
```

---

## 🔒 Безопасность и авторизация

### Включение защиты паролем

Запустите сервер с флагом `--httpauth`:
```bash
./torrserver --httpauth
```

Первый пользователь создаётся через CLI:
```bash
./torrserver auth add admin
# Введите пароль (будет запрошен скрытно)
```

### Авторизация при обращении к серверу

CLI автоматически запросит пароль, если вы указали `--user`, но не указали `--pass`:
```bash
./torrserver --user admin torrents list
# Password: <ввод скрыт>
```

**Для скриптов и CI/CD** используйте переменные окружения:
```bash
export TS_USER=admin
export TS_PASSWORD=MySecretPass123

./torrserver torrents list  # Без запроса пароля
```

Приоритет параметров CLI: явные флаги, переменные окружения, выбранный context, затем значения по умолчанию.
Для shutdown token используйте `TS_SHUTDOWN_TOKEN`; это безопаснее, чем передавать секрет через `--token`.

> ⚠️ **Важно:** Не передавайте password или token через `--pass`/`--token` — они видны в списке процессов
> (`ps aux`). Используйте `TS_PASSWORD` и `TS_SHUTDOWN_TOKEN`.

### Shutdown Token

Для защиты от случайного выключения сервера:
```bash
# Проверить, настроен ли token; его значение сервер никогда не возвращает
./torrserver config shutdown-token status

# Сгенерировать, сохранить на сервере и сразу поместить единственный вывод в environment
export TS_SHUTDOWN_TOKEN="$(./torrserver config shutdown-token generate --yes)"

# Остановить сервер с токеном
./torrserver shutdown --mode public
```

Для установки заранее подготовленного значения задайте `TS_SHUTDOWN_TOKEN` и выполните
`./torrserver config shutdown-token set --yes`. Команды `generate` и `set` заменяют действующий token, поэтому без
`--yes` требуют интерактивного подтверждения. Новый сгенерированный token выводится только один раз; `status` и `set`
никогда его не печатают.

---

## 🌐 Работа с несколькими серверами (Контексты)

Вы можете управлять несколькими серверами TorrServer (локальным и удаленными) с одного компьютера.

### 1. Добавление сервера
Дайте серверу имя и укажите его адрес:
```bash
./torrserver context add --name home --server http://192.168.1.50:8090
```

### 2. Использование
Вы можете выполнить команду для конкретного сервера, используя флаг `--context`:

**Добавить торрент на удаленный сервер:**
```bash
./torrserver --context home torrents add "magnet:?xt=urn:btih:..." --title "Movie" --save

# Файл читается на машине с CLI и загружается на сервер из контекста home
./torrserver --context home torrents add ./movie.torrent --save
```

**Получить ссылку на стрим с удаленного сервера:**
```bash
./torrserver --context home url "Movie" --file "1080p"
```

### 3. Переключение по умолчанию
Если вы хотите, чтобы все команды выполнялись на удаленном сервере без постоянного указания флага, переключите контекст:

```bash
# Переключиться на сервер 'home'
./torrserver context use --name home

# Теперь эта команда сработает на 192.168.1.50
./torrserver torrents list

# Вернуться на локальный сервер
./torrserver context use --name local
```

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
make build          # Бинарник
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
| `--path` | `./` | Путь к конфигурации (`config.yml` и БД) |
| `--torrentsdir` | `./` | Папка для торрент-файлов и кэша |
| `--logpath` | `./` | Путь для логов |
| `--httpauth` | `false` | Включить защиту паролем |

### Переменные окружения (Docker)

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `TS_PORT` | `8090` | HTTP порт |
| `TS_DLN` | `1` | DLNA (1/0) |
| `TS_CONF_PATH` | `/opt/ts/config` | Путь к конфигу |
| `TS_TORR_DIR` | `/opt/ts/torrents` | Путь к торрентам |
| `TS_CACHE_SIZE` | `67108864` | Кэш (64 MB) |
| `TS_USER` | `` | Логин для авторизации |
| `TS_PASSWORD` | `` | Пароль для авторизации |
| `TS_SHUTDOWN_TOKEN` | `` | Токен для shutdown (public mode) |

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
