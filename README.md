# TorrServer v3.0 — Минималистичное Ядро Потоковой Передачи

> **Поставил и забыл** — лёгкий, быстрый, без лишнего

[![GitHub release](https://img.shields.io/github/v/release/YouROK/TorrServer)](https://github.com/YouROK/TorrServer/releases)
[![Docker Pulls](https://img.shields.io/docker/pulls/yourok/torrserver)](https://hub.docker.com/r/yourok/torrserver)
[![License](https://img.shields.io/github/license/YouROK/TorrServer)](https://github.com/YouROK/TorrServer/blob/master/LICENSE)

---

## 📖 Описание

**TorrServer v3.0** — это минималистичное ядро для потоковой передачи торрентов.

**Философия:**
- ✅ **Лёгкость** — RAM ~30-50MB idle
- ✅ **Оптимизация** — быстрый запуск (~1-2 сек)
- ✅ **Скорость** — минимальные задержки при стриминге
- ✅ **Удобство** — простая настройка, работа "из коробки"

**Что это:**
- Сервер для стриминга торрентов через HTTP
- DLNA сервер для TV (Kodi, VLC, WebOS, Tizen)
- M3U плейлисты для любых плееров
- HTTP API для скриптов и автоматизации
- FUSE/WebDAV для монтирования как диск

**Что это НЕ:**
- ❌ Онлайн-кинотеатр с веб-интерфейсом
- ❌ Поисковик торрентов (пользователь ищет сам)
- ❌ Мультипользовательская система

---

## 🚀 Быстрый Старт

### Docker (Рекомендуется)

```bash
docker run -d \
  --name torrserver \
  -p 8090:8090 \
  -p 9080:9080 \
  -v ./config:/opt/ts/config \
  -v ./torrents:/opt/ts/torrents \
  -e TS_PORT=8090 \
  -e TS_DLN=1 \
```

**Порты:**
- `8090` — HTTP API
- `9080` — DLNA (для TV)

---

## 📺 Как Смотреть

### Вариант 1: DLNA (TV)

```
1. Открыть Kodi/VLC на TV
2. Перейти в раздел DLNA
3. Найти "TorrServer"
4. Выбрать торрент → файл → Play
```

**Поддерживаемые устройства:**
- Kodi (все платформы)
- VLC (PC, Mobile, TV)
- LG WebOS TV
- Samsung Tizen TV
- PlayStation/Xbox Media Player

---

### Вариант 2: M3U Плейлист

**Получить плейлист:**
```bash
curl http://localhost:8090/playlistall/all.m3u
```

**Открыть в VLC:**
```
VLC → Media → Open Network Stream
http://localhost:8090/playlistall/all.m3u
```

**Открыть в Kodi:**
```
Kodi → Videos → Files → Add videos
URL: http://localhost:8090/playlistall/all.m3u
```

---

### Вариант 3: HTTP API

**Добавить торрент:**
```bash
curl -X POST http://localhost:8090/torrents \
  -H "Content-Type: application/json" \
  -d '{
    "action": "add",
    "link": "magnet:?xt=urn:btih:...",
    "save_to_db": true
  }'
```

**Получить список:**
```bash
curl -X POST http://localhost:8090/torrents \
  -H "Content-Type: application/json" \
  -d '{"action": "list"}'
```

**Стриминг:**
```bash
vlc "http://localhost:8090/stream?link=magnet:?xt=urn:btih:..."
```

**Полная документация API:** [API.md](docs/API.md)

---

## 🔧 Конфигурация

### Переменные Окружения

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `TS_PORT` | `8090` | HTTP порт API |
| `TS_DLN` | `1` | Включить DLNA (1/0) |
| `TS_CONF_PATH` | `/opt/ts/config` | Путь к конфигурации |
| `TS_TORR_DIR` | `/opt/ts/torrents` | Путь к торрентам |
| `TS_FRIENDLY_NAME` | `TorrServer` | Имя DLNA сервера |
| `TS_CACHE_SIZE` | `67108864` | Размер кэша (64 MB) |
| `TS_CONNECTIONS_LIMIT` | `5` | Лимит подключений |

### API Настройки

**Получить настройки:**
```bash
curl -X POST http://localhost:8090/settings \
  -H "Content-Type: application/json" \
  -d '{"action": "get"}'
```

**Изменить настройки:**
```bash
curl -X POST http://localhost:8090/settings \
  -H "Content-Type: application/json" \
  -d '{
    "action": "set",
    "sets": {
      "CacheSize": 134217728,
      "ConnectionsLimit": 10,
      "EnableDLNA": true
    }
  }'
```

---

## 📁 Тома Docker

```yaml
volumes:
  - ./config:/opt/ts/config      # Конфигурация (BBolt БД)
  - ./torrents:/opt/ts/torrents  # Торренты (кэш)
  - ./logs:/opt/ts/log           # Логи (опционально)
```

---

## 🛠️ Сборка из Исходников

### Требования
- Go 1.22+
- Docker (опционально)

### Сборка

```bash
cd server
go build -o ../dist/torrserver ./cmd
```

### Docker

```bash
docker build -t torrserver .
docker-compose -f docker-compose.light.yml up -d
```

---

## 📊 Архитектура

```
┌─────────────────────────────────────────────────────────────┐
│                    TorrServer Core                           │
│                                                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌──────────────┐ │
│  │  HTTP API       │  │   DLNA Server   │  │  M3U Gen     │ │
│  │  :8090          │  │   :9080         │  │  (в API)     │ │
│  └────────┬────────┘  └────────┬────────┘  └──────┬───────┘ │
└───────────┼────────────────────┼──────────────────┼─────────┘
            │                    │                  │
     ┌──────┴──────┐      ┌─────┴─────┐    ┌──────┴──────┐
     ▼             ▼      ▼           ▼    ▼             ▼
┌─────────┐  ┌─────────┐  ┌───────┐  ┌───────┐  ┌───────────┐
│         │  │  TUI    │  │ Kodi  │  │  VLC  │  │Any Player │
│   API   │  │  (CLI)  │  │(DLNA) │  │ (M3U) │  │  (HTTP)   │
└─────────┘  └─────────┘  └───────┘  └───────┘  └───────────┘
```

---

## 🔍 Поиск и Добавление Торрентов

TorrServer v3.0 **не включает** встроенный поиск торрентов.

**Как добавлять:**

### 1. Найти торрент самостоятельно
- RuTracker.org
- NNMClub.to
- Rutor.info
- Другие трекеры

### 2. Скопировать magnet ссылку

### 3. Добавить через API
```bash
curl -X POST http://localhost:8090/torrents \
  -H "Content-Type: application/json" \
  -d '{
    "action": "add",
    "link": "magnet:?xt=urn:btih:...",
    "save_to_db": true
  }'
```

### 4. Или через TUI (планируется)
```bash
torrserver add --magnet "magnet:?xt=urn:btih:..."
```

---

## API Совместимость

TorrServer v3.0 сохраняет обратную совместимость с API YouROK/TorrServer за исключением удалённых endpoints.

### Совместимые Endpoints

| Endpoint | Методы | Auth | Описание |
|----------|--------|------|----------|
| `/stream` | GET, HEAD | Нет | Стриминг торрента |
| `/stream/*fname` | GET, HEAD | Нет | Стриминг файла |
| `/play/:hash/:id` | GET, HEAD | Нет | Воспроизведение файла |
| `/settings` | POST | Да | Настройки |
| `/torrents` | POST | Да | Управление торрентами |
| `/torrent/upload` | POST | Да | Загрузка .torrent файла |
| `/cache` | POST | Да | Управление кэшем |
| `/viewed` | POST | Да | История просмотров |
| `/playlist` | GET | Нет | M3U плейлист |
| `/playlistall/all.m3u` | GET | Да | Все плейлисты |
| `/download/:size` | GET | Да | Скачивание |
| `/torznab/search/*query` | GET | conditional* | Поиск (Torznab) |
| `/storage/settings` | GET, POST | Да | Настройки хранилища |
| `/shutdown` | GET | Да | Остановка сервера |

*\* `/torznab/search` - публичный если `SearchWA=true`, иначе требует авторизацию.*

### Новые Endpoints (V2)

| Endpoint | Методы | Auth | Описание |
|----------|--------|------|----------|
| `/api/version` | GET | Нет | Версия API |
| `/api/v1/*` | * | * | Новый API v1 namespace |
| `/streams/stat` | GET, HEAD | Нет | Статистика стрима |
| `/streams/m3u` | GET, HEAD | Нет | M3U плейлист |
| `/streams/play` | GET, HEAD | Нет | API воспроизведения |
| `/streams/save` | POST | Да | Сохранить торрент |

### Удалённые Endpoints

| Endpoint | Причина |
|----------|---------|
| `/search/*query` | Поиск Rutor удалён (экономия RAM) |

### Route Manifest

Полный список маршрутов: [server/docs/route-manifest.json](server/docs/route-manifest.json)

---

## 🧩 Интеграции

### Sonarr/Radarr

Используйте Torznab API:

```
URL: http://localhost:8090/torznab/search?query=Avatar
API Key: (не требуется)
```

### TUI/CLI (Планируется)

```bash
torrserver search "Avatar 2022"
torrserver add --magnet "..."
torrserver list
torrserver remove <hash>
```

---

## 📈 Потребление Ресурсов

| Режим | RAM | CPU | Время запуска |
|-------|-----|-----|---------------|
| **Idle** | ~30-50 MB | ~0.1% | ~1-2 сек |
| **Стриминг** | ~100-200 MB | ~5-10% | - |

---

## ❓ FAQ

### Q: Где веб-интерфейс?
**A:** TorrServer v3.0 не включает веб-интерфейс. Используйте:
- DLNA (Kodi/VLC на TV)
- M3U плейлисты (VLC, Kodi)
- HTTP API (скрипты)
- TUI (планируется)

### Q: Как искать торренты?
**A:** TorrServer не включает поиск. Ищите на трекерах, добавляйте через API.

### Q: Работает ли DLNA через интернет?
**A:** Нет, DLNA работает только в локальной сети. Для удалённого доступа используйте M3U/API.

### Q: Можно ли вернуть поиск Rutor?
**A:** Нет, поиск удалён для экономии ресурсов (100-200MB RAM).

### Q: Где Telegram бот?
**A:** Удалён в v3.0 для упрощения кода.

---

## 🗺️ Roadmap

### v3.0 (Текущая)
- ✅ Удаление Web UI
- ✅ Удаление Rutor поиска
- ✅ Удаление метрик
- ✅ Удаление Telegram бота
- ✅ Упрощение Docker
- ✅ Рефакторинг server/cmd/

### v3.1 (Планируется)
- [ ] TUI (Terminal UI)
- [ ] CLI утилита
- [ ] YAML конфиг
- [ ] Watch Folder (автодобавление)

### v3.2 (Будущее)
- [ ] Sonarr/Radarr интеграция
- [ ] Kodi addon
- [ ] Desktop приложение

---

## 🤝 Contributing

Приветствуются:
- ✅ Баг репорты
- ✅ Предложения по улучшению
- ✅ Документация
- ✅ Тесты

**Как помочь:**
1. Fork проекта
2. Создать feature branch
3. Внести изменения
4. Отправить Pull Request

---

## 📄 Лицензия

GPL 3.0 — см. [LICENSE](LICENSE)

---

## 🔗 Ссылки

- [GitHub Releases](https://github.com/YouROK/TorrServer/releases)
- [Docker Hub](https://hub.docker.com/r/yourok/torrserver)
- [API Документация](docs/API.md)
- [Issues](https://github.com/YouROK/TorrServer/issues)

---

*Документ создан: 2026-03-30*  
*Версия: 3.0.0*
