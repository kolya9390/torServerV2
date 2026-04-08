# TorrServer — Ядро Потоковой Передачи Торрентов

[![Build Status](https://github.com/kolya9390/torServerV2/actions/workflows/ci.yml/badge.svg)](https://github.com/kolya9390/torServerV2/actions)
[![License](https://img.shields.io/github/license/YouROK/TorrServer)](LICENSE)

> **Поставил и забыл** — лёгкий, быстрый, без лишнего

---

## ℹ️ О проекте

Этот проект основан на [YouROK/TorrServer](https://github.com/YouROK/TorrServer) — оригинальной реализации сервера для стриминга торрентов.

**Рекомендуемый клиент:**
Для удобного управления сервером, добавления торрентов и выбора плеера используйте **[TorrServe](https://github.com/YouROK/TorrServe)** для Android.
Он позволяет:
- Управлять настройками сервера.
- Добавлять торренты и управлять кэшем.
- Выбирать плеер для воспроизведения.

**Совместимость:**
- ✅ **Полностью совместим** с клиентом [YouROK/TorrServe](https://github.com/YouROK/TorrServe)
- ✅ **Протестирован** для просмотра торрентов и стриминга
- ✅ **API совместим** с оригинальной версией

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

### Вариант 1: DLNA (TV)

1. Открыть Kodi/VLC на TV
2. Найти "TorrServer" в DLNA
3. Выбрать торрент → Play

### Вариант 2: M3U плейлист

```bash
curl http://localhost:8090/playlistall/all.m3u
```

Открыть URL в VLC/Kodi.

### Вариант 3: HTTP API

```bash
# Добавить торрент
curl -X POST http://localhost:8090/torrents \
  -H "Content-Type: application/json" \
  -d '{"action": "add", "link": "magnet:?xt=urn:btih:..."}'

# Стриминг
vlc "http://localhost:8090/stream?link=magnet:?xt=urn:btih:..."
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
