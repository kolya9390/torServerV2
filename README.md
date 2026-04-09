# TorrServer — Ядро Потоковой Передачи Торрентов

[![Build Status](https://github.com/kolya9390/torServerV2/actions/workflows/ci.yml/badge.svg)](https://github.com/kolya9390/torServerV2/actions)
[![License](https://img.shields.io/github/license/YouROK/TorrServer)](LICENSE)

> **Поставил и забыл** — лёгкий, быстрый, без лишнего

---

## ℹ️ О проекте

Этот проект основан на [YouROK/TorrServer](https://github.com/YouROK/TorrServer) — оригинальной реализации сервера для стриминга торрентов.

**Рекомендуемые клиенты для просмотра:**

- **[TorrServe](https://github.com/YouROK/TorrServe)** — Android-клиент для управления сервером, поиска торрентов, добавления и выбора плеера.
- **[Lampa](https://github.com/yumata/lampa-source)** — приложение для Smart TV (WebOS, Tizen, Android TV). Подключается к TorrServer через плагин и позволяет искать и смотреть торренты прямо на телевизоре.

**Совместимость:**
- ✅ **Протестирован** с TorrServe, Lampa, VLC, MPV, Kodi
- ✅ **API совместим** с оригинальной версией YouROK/TorrServer
- ✅ **DLNA** работает с любыми DLNA-клиентами (TV, плееры)

---

## 📖 Что это?

**TorrServer** — минималистичное ядро для стриминга торрентов через HTTP/DLNA.

**Философия:**
- ✅ **Лёгкость** — RAM ~30-50MB idle
- ✅ **Скорость** — запуск ~1-2 сек
- ✅ **Удобство** — настройка "из коробки"

**Что умеет:**
- Стриминг торрентов через HTTP
- DLNA сервер для TV (Kodi, VLC, WebOS, Tizen)
- M3U плейлисты
- HTTP API для автоматизации
- FUSE/WebDAV для монтирования

---

## 🚀 Быстрый старт

### 1. Бинарный файл (Linux / macOS / Windows)
Скачайте файл с [страницы релизов](https://github.com/kolya9390/torServerV2/releases) и запустите:

```bash
# Запуск сервера (по умолчанию порт 8090)
./torrserver

# Запуск с настройками
./torrserver --port 8090 --path ./config --torrentsdir ./torrents
```
*(Для Windows используйте `torrserver.exe`)*

### 2. Docker (Рекомендуется)

```bash
docker run -d \
  --name torrserver \
  -p 8090:8090 \
  -p 9080:9080 \
  -v ./config:/opt/ts/config \
  -v ./torrents:/opt/ts/torrents \
  ghcr.io/kolya9390/torServerV2:latest
```

### 3. Docker Compose
Если вы склонировали репозиторий:
```bash
docker compose -f docker-compose.yml up -d
```

---

## 📺 Как смотреть

### Шаг 1: Добавить торрент

**Через magnet-ссылку:**
```bash
curl -X POST http://localhost:8090/torrents \
  -H "Content-Type: application/json" \
  -d '{"action": "add", "link": "magnet:?xt=urn:btih:HASH&dn=Title"}'
```

**Через .torrent файл (надёжнее):**
```bash
curl -X POST http://localhost:8090/torrents \
  -H "Content-Type: application/json" \
  -d '{"action": "add", "link": "file:///path/to/file.torrent"}'
```

### Шаг 2: Проверить статус

```bash
curl -X POST http://localhost:8090/torrents \
  -H "Content-Type: application/json" \
  -d '{"action": "list"}'
```

Дождитесь статуса **`Working`** (код 3). Пока статус `Torrent added` (код 0) — сервер ищет пиров.

### Шаг 3: Запустить просмотр

**Через MPV:**
```bash
mpv --no-ytdl 'http://localhost:8090/streams/play?link=HASH&index=1'
```

**Через VLC:**
```bash
open -a VLC 'http://localhost:8090/streams/play?link=HASH&index=1'
```

> **Важно:** `--no-ytdl` для MPV обязателен — отключает youtube-dl, который вызывает зависания.

---

## 📡 API Examples

Base URL: `http://localhost:8090`

### Torrent Management

**List all torrents:**
```bash
curl -X POST http://localhost:8090/torrents \
  -H "Content-Type: application/json" \
  -d '{"action": "list"}'
```

**Add a torrent (magnet):**
```bash
curl -X POST http://localhost:8090/torrents \
  -H "Content-Type: application/json" \
  -d '{
    "action": "add",
    "link": "magnet:?xt=urn:btih:HASH&dn=Title"
  }'
```

**Add a torrent (.torrent file):**
```bash
curl -X POST http://localhost:8090/torrents \
  -H "Content-Type: application/json" \
  -d '{"action": "add", "link": "file:///path/to/file.torrent"}'
```

**Remove a torrent:**
```bash
curl -X POST http://localhost:8090/torrents \
  -H "Content-Type: application/json" \
  -d '{"action": "rem", "hash": "HASH"}'
```

**Remove all torrents:**
```bash
curl -X POST http://localhost:8090/torrents \
  -H "Content-Type: application/json" \
  -d '{"action": "wipe"}'
```

### Streaming

**Play specific file (recommended):**
```bash
mpv --no-ytdl 'http://localhost:8090/streams/play?link=HASH&index=1'
```

**M3U Playlist:**
```bash
curl http://localhost:8090/playlistall/all.m3u
```

### Settings

**Get settings:**
```bash
curl -X POST http://localhost:8090/settings \
  -H "Content-Type: application/json" \
  -d '{"action": "get"}'
```

**Update settings:**
```bash
curl -X POST http://localhost:8090/settings \
  -H "Content-Type: application/json" \
  -d '{"action": "set", "sets": {"CacheSize": 134217728}}'
```

---

## 🛠️ Сборка

```bash
# Бинарник
make build

# Тесты
make test

# Моки
make generate-mocks

# Docker
docker build -t torrserver .
```

**Требования:** Go 1.26+

---

## ⚙️ Конфигурация

### Флаги запуска

| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `--port` | `8090` | Порт веб-интерфейса и API |
| `--path` | `./` | Путь к папке с конфигурацией (`config.yml` и БД) |
| `--torrentsdir` | `./` | Папка для хранения торрент-файлов и кэша |
| `--logpath` | `./` | Путь для сохранения логов |
| `--httpauth` | `false` | Включить защиту паролем (файл `accs.db`) |

### Переменные окружения (для Docker)

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `TS_PORT` | `8090` | HTTP порт |
| `TS_DLN` | `1` | DLNA (1/0) |
| `TS_CONF_PATH` | `/opt/ts/config` | Путь к конфигу |
| `TS_TORR_DIR` | `/opt/ts/torrents` | Путь к торрентам |
| `TS_CACHE_SIZE` | `67108864` | Кэш (64 MB) |

---

## 📊 Ресурсы

| Режим | RAM | CPU |
|-------|-----|-----|
| **Idle** | ~30-50 MB | ~0.1% |
| **Стриминг** | ~100-200 MB | ~5-10% |

---

## 📄 Лицензия

GPL 3.0 — см. [LICENSE](LICENSE)
