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

**TorrServer** — минималистичное ядро для стриминга торрентов через HTTP/DLNA. Один бинарник содержит и сервер, и CLI для управления.

**Философия:**
- ✅ **Лёгкость** — RAM ~30-50MB idle
- ✅ **Скорость** — запуск ~1-2 сек
- ✅ **Удобство** — настройка "из коробки"

**Что умеет:**
- Стриминг торрентов через HTTP
- DLNA сервер для TV (Kodi, VLC, WebOS, Tizen)
- M3U плейлисты
- HTTP API для автоматизации
- CLI для управления сервером

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
  ghcr.io/kolya9390/torServerV2:latest
```

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

### Основные команды:

**Торренты:**
```bash
# Список торрентов (обратите внимание на колонку # — это индекс торрента)
./torrserver torrents list

# Добавить торрент
./torrserver torrents add --link "magnet:?xt=urn:btih:..." --title "Movie" --save
./torrserver torrents add --link "file:///path/to/file.torrent" --save

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
./torrserver --context home torrents add --link "magnet:?xt=urn:btih:..." --title "Movie" --save
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

### Переменные окружения (Docker)

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
