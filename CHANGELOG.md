# Changelog

Значимые пользовательские изменения TorrServerV2 документируются в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/), версии следуют
[Semantic Versioning](https://semver.org/lang/ru/). В release notes попадает только секция точной публикуемой версии;
`Unreleased` никогда не публикуется автоматически.

## [Unreleased]

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

[Unreleased]: https://github.com/kolya9390/torServerV2/commits/main
