# Changelog

Значимые пользовательские изменения TorrServerV2 документируются в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/), версии следуют
[Semantic Versioning](https://semver.org/lang/ru/). В release notes попадает только секция точной публикуемой версии;
`Unreleased` никогда не публикуется автоматически.

## [Unreleased]

### Added

- Release artifacts теперь включают отдельные `torrserver` и `torrctl` для всех поддерживаемых платформ.

### Changed

- CI и tag release требуют успешный split-process smoke test; Docker image запускает только `torrserver serve`.

### Fixed

### Security

### Deprecated

### Removed

## [1.0.0-beta.3] - 2026-07-16

### Added

- Добавлен единый CLI-сценарий для просмотра списка торрентов, добавления magnet или `.torrent` файла и получения
  stream URL в human-readable и JSON-форматах.
- Добавлены проверяемые build metadata и aggregate SHA-256 checksums для release binaries.

### Changed

- Release artifacts получают детерминированные имена с версией, операционной системой и архитектурой.
- Docker-образы публикуются только из release tags; `latest` зарезервирован для stable-релизов.

### Fixed

- CLI ошибки теперь сохраняют стабильные exit codes и не раскрывают token, password, URL credentials или
  произвольное тело ответа сервера.
- Shutdown и повторное подключение к stream проходят через явный lifecycle и ограниченные ожидания.

### Security

- Управление shutdown token вынесено в явные CLI-команды с подтверждением destructive-операций и скрытым выводом
  секретов.
- Debug endpoints, profiling и расширенные stream diagnostics остаются выключенными при `debug.enabled: false`.

### Deprecated

- Нет.

### Removed

- Нет.

### Known limitations

- Контролируемый прогон одного heavy 4K stream по A175 выполнен, но результат должен повторно подтверждаться beta
  gate A194 и пока не является stable-гарантией.
- Два одновременных уникальных heavy 4K stream являются целевым beta-сценарием, но пока не stable guarantee.
- Playback зависит от sustained bitrate, swarm health, WAN/LAN и клиента; слабая раздача не может быть исправлена
  только серверным scheduling.

[Unreleased]: https://github.com/kolya9390/torServerV2/compare/v1.0.0-beta.3...HEAD
[1.0.0-beta.3]: https://github.com/kolya9390/torServerV2/compare/v1.0.0-beta.2...v1.0.0-beta.3
