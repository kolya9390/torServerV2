# Версионирование TorrServerV2

## Назначение

TorrServerV2 использует [Semantic Versioning 2.0.0](https://semver.org/) для публичных релизов. Версия описывает
совместимость внешнего контракта, а не объем кода, возраст проекта или процент завершенности.

Единственный источник release-версии - неизменяемый git tag. В имени тега используется традиционный префикс `v`,
хотя сама SemVer-версия начинается с цифры.

Поддерживаемые формы:

```text
vMAJOR.MINOR.PATCH
vMAJOR.MINOR.PATCH-alpha.N
vMAJOR.MINOR.PATCH-beta.N
vMAJOR.MINOR.PATCH-rc.N
```

Примеры: `v1.0.0-beta.1`, `v1.0.0-rc.2`, `v1.0.0`, `v1.1.0`, `v1.1.1`.

Release tags не содержат build metadata с `+`. Commit, build time и dirty state относятся к идентификации бинарника,
но не создают новую release-версию. Опубликованный тег никогда не перемещается и не переиспользуется. Исправление
опубликованного релиза получает новую версию.

## Build metadata

`make build`, `make build-all`, Docker и GitHub Actions передают бинарнику одинаковые поля: application version,
commit, build time и dirty state. Локально version вычисляется через `git describe --tags --always`; измененное
дерево, включая untracked files, получает явный суффикс `-dirty`. Source archive без `.git` собирается как
`dev`, `commit=unknown`, `dirty=unknown` и не выдает себя за опубликованный релиз.

Automation может детерминированно переопределить все поля:

```bash
VERSION=v1.0.0-beta.1 \
COMMIT=0123456789abcdef \
BUILD_TIME=2026-07-15T08:00:00Z \
DIRTY=clean \
make build
```

`BUILD_TIME` по умолчанию равен `unknown`: текущее время намеренно не добавляется, чтобы две сборки одного commit с
одинаковым toolchain могли совпасть. Если время необходимо для release audit, automation передает заранее
зафиксированное UTC-значение в RFC 3339; при сравнении reproducibility оно должно быть одинаковым для всех платформ.
Допустимые значения metadata не содержат пробелов, что исключает неоднозначный разбор linker flags.

## Проверка release artifacts

Release публикует бинарники с детерминированными именами
`torrserver-<version>-<os>-<arch>[.exe]`, один aggregate manifest
`torrserver-<version>-SHA256SUMS`. Release не создается, если отсутствует хотя бы один Linux amd64/arm64, macOS
amd64/arm64 или Windows amd64 artifact либо checksum verification не проходит.

На Linux выбранный файл проверяется так:

```bash
asset=torrserver-1.0.0-beta.1-linux-amd64
grep "  ${asset}$" torrserver-1.0.0-beta.1-SHA256SUMS | sha256sum --check -
```

На macOS используется системный `shasum`:

```bash
asset=torrserver-1.0.0-beta.1-darwin-arm64
grep "  ${asset}$" torrserver-1.0.0-beta.1-SHA256SUMS | shasum -a 256 --check -
```

В Windows PowerShell:

```powershell
$asset = "torrserver-1.0.0-beta.1-windows-amd64.exe"
$line = Select-String -Path "torrserver-1.0.0-beta.1-SHA256SUMS" -Pattern "  $([regex]::Escape($asset))$"
$expected = ($line.Line -split "\s+")[0].ToLowerInvariant()
$actual = (Get-FileHash -Algorithm SHA256 $asset).Hash.ToLowerInvariant()
if ($actual -ne $expected) { throw "SHA-256 mismatch" }
```

## Значение чисел

- `MAJOR` увеличивается при несовместимом изменении публичного контракта после `v1.0.0`.
- `MINOR` увеличивается при добавлении обратно совместимой публичной возможности.
- `PATCH` увеличивается при обратно совместимом исправлении дефекта или безопасности.

После увеличения `MAJOR` значения `MINOR` и `PATCH` сбрасываются в ноль. После увеличения `MINOR` значение `PATCH`
сбрасывается в ноль.

Prerelease имеет меньший приоритет, чем соответствующая стабильная версия. Для проекта принят порядок зрелости:

```text
alpha.1 < alpha.2 < beta.1 < beta.2 < rc.1 < rc.2 < stable
```

## Публичный контракт

SemVer защищает наблюдаемое и документированное поведение следующих поверхностей:

1. HTTP API `/api/v1`: методы, маршруты, авторизация, обязательные поля, типы, статусы ошибок и JSON-формы.
2. CLI: имена команд и флагов, exit codes, документированное поведение и стабильный machine-readable JSON.
3. `config.yml`: имена, типы, единицы измерения, значения по умолчанию и семантика параметров.
4. Документированные переменные окружения и их приоритет относительно CLI/context/defaults.
5. Авторизация, управление shutdown token и правила локального/удаленного shutdown.
6. Поддерживаемые форматы persistent storage и автоматические миграции без потери данных.
7. Документированные stream URL, file index, Range/reconnect и совместимость поддерживаемых клиентов.

HTTP API version и версия приложения независимы. Приложение `v1.3.0` может продолжать предоставлять `/api/v1`, пока
этот API остается совместимым. Версия torrent engine также выводится отдельно и не определяет версию приложения.

Случайное или недокументированное поведение не становится контрактом автоматически. Если на него фактически
полагаются поддерживаемые клиенты, изменение сначала рассматривается как compatibility risk и проверяется тестами.

## Внутренние изменения

Следующие изменения сами по себе не требуют повышения `MAJOR`, если сохраняют публичный контракт:

- DI wiring, package boundaries, интерфейсы и внутренние DTO;
- алгоритмы cache, preload, scheduling, fairness и admission control;
- конкурентная реализация, goroutine ownership, locks и worker pools;
- внутренняя torrent-library integration и обновление зависимости;
- уровни логирования, debug-only metrics и profiling implementation;
- оптимизация CPU, RAM, allocations, disk I/O и network I/O;
- рефакторинг тестов, mocks, CI и build scripts.

Если такое изменение меняет документированный default, лимит, формат ошибки, CLI/API/config или persistent data,
версия определяется уже по внешнему эффекту.

## Каналы релизов

### Alpha

Alpha предназначена для незавершенных экспериментов. Возможны отсутствующие функции, изменение контрактов и
непригодность к миграции. Alpha не публикуется как Docker `latest` и не получает stable aliases.

### Beta

Текущий канал TorrServerV2 - beta. Ближайшая планируемая версия: `v1.0.0-beta.1`.

Beta означает:

- основные пользовательские сценарии реализованы и пригодны для реального тестирования;
- известные ограничения и неподтвержденные гарантии перечислены в release notes;
- compatibility changes еще допустимы, но должны быть намеренными, протестированными и документированными;
- security, data loss, startup, shutdown, torrent add и stream URL blockers не допускаются к публикации;
- beta не получает stable Docker aliases и не считается production-stable.

Следующая итерация той же линии получает `beta.2`, `beta.3` и так далее. Опубликованная beta не удаляется и не
перемещается только потому, что в ней найден дефект.

### Release Candidate

`v1.0.0-rc.1` разрешен после feature freeze. HTTP API, CLI, config и storage contract заморожены; допускаются только
исправления release blockers, документации и доказанные безопасные оптимизации. Несовместимое изменение после RC
начинает новую RC-последовательность и требует явных migration notes.

### Stable

`v1.0.0` начинает публичную гарантию совместимости. Stable требует выполненных release gates, проверенных миграций,
отсутствия известных критических дефектов и опубликованных checksums/release notes. После `v1.0.0` несовместимое
изменение публичного контракта требует следующего `MAJOR`.

## Docker-каналы

Образы публикуются в `ghcr.io/kolya9390/torserverv2`. Канал определяется только проверенным Git ref; вручную
перемещать release-теги или дописывать stable aliases запрещено.

| Источник | Публикуемые теги | Назначение |
|---|---|---|
| `v1.0.0-alpha.1` | `1.0.0-alpha.1` | Точная alpha без floating aliases |
| `v1.0.0-beta.1` | `1.0.0-beta.1` | Точная beta без floating aliases |
| `v1.0.0-rc.1` | `1.0.0-rc.1` | Точный RC без floating aliases |
| `v1.2.3` | `1.2.3`, `1.2`, `1`, `latest` | Stable после успешного release gate |

`major`, `major.minor` и `latest` являются плавающими stable aliases. Для воспроизводимого развертывания используйте
точную версию, а для строгой фиксации - digest:

```bash
docker pull ghcr.io/kolya9390/torserverv2:1.0.0-beta.1
docker image inspect ghcr.io/kolya9390/torserverv2:1.0.0-beta.1 \
  --format '{{index .RepoDigests 0}}'
docker pull ghcr.io/kolya9390/torserverv2@sha256:<digest>
```

Rollback выполняется изменением deployment или Compose-конфигурации на предыдущий точный тег либо сохраненный
digest с последующим пересозданием контейнера. Опубликованный SemVer-тег не перемещается и не перезаписывается для
rollback: исправление выпускается новой версией. Перед откатом необходимо проверить совместимость config и storage;
если релиз содержит необратимую миграцию, используется документированная для него процедура восстановления данных.

Release-образ публикуется только после того, как GitHub Release workflow проверил формат тега, release notes, тесты,
lint, полную матрицу сборки и создал GitHub Release. Alpha, beta и RC не способны получить `latest`, `major` или
`major.minor` aliases.

## Pre-1.0 дисциплина

Prerelease-линия `v1.0.0-*` формально еще не является стабильным `v1.0.0`. Это не разрешение менять все без правил:

- каждое compatibility change должно иметь причину, тесты и release note;
- конфигурация и storage по возможности мигрируются автоматически;
- удаление уже используемой возможности предваряется deprecation, если это не устраняет критическую уязвимость;
- beta и RC никогда не маскируются под stable или Docker `latest`;
- known issues публикуются честно, включая неподтвержденные performance guarantees.

## Deprecation и миграции

После `v1.0.0` обратно несовместимое удаление проходит через deprecation в совместимом `MINOR` релизе. Deprecated
API/CLI/config продолжает работать в объявленное окно, возвращает или документирует замену и удаляется только в
следующем `MAJOR`. Конкретная дата sunset может дополнительно ограничивать legacy HTTP route, но не заменяет release
notes и migration guide.

Для stable-линии предупреждение публикуется не менее чем в одном предшествующем `MINOR` релизе и не менее чем за
90 дней до удаления; применяется более длинное окно. Даже после этого удаление публичного контракта требует нового
`MAJOR`. Для prerelease-линии окно может быть короче, но несовместимое изменение всё равно получает отдельную запись
`Deprecated` или `Removed`, migration note и следующую prerelease-версию.

Deprecation должна быть наблюдаемой на затронутой поверхности:

| Поверхность | Сигнал и переход |
|---|---|
| HTTP API | Документированная замена; при доработке route - стандартные `Deprecation`, `Sunset` и `Link`, если применимо |
| CLI | Warning в `stderr`, сохранение чистого JSON в `stdout`, replacement command/flag |
| Config и environment | Старое имя временно читается с warning, новое имеет явный приоритет |
| Persistent data | Версионированная идемпотентная migration, backup/rollback и проверка результата |
| Streaming contract | Compatibility test для старого клиента и описание изменения reconnect/range semantics |

Storage migration должна быть идемпотентной, проверяемой и не уничтожать исходные данные до успешной верификации.
Если автоматическая миграция невозможна, релиз не считается обратно совместимым без отдельного migration tool и
rollback procedure.

Критическое security-исправление может безопасно запретить уязвимое поведение в `PATCH`, даже если это влияет на
часть пользователей. Такое исключение требует security note, безопасного replacement path и явного описания риска.

## Примеры решений

| Изменение | Версия после stable | Обоснование |
|---|---:|---|
| Исправление memory leak без изменения поведения | `PATCH` | Совместимое исправление |
| Новая необязательная CLI-команда | `MINOR` | Новая совместимая возможность |
| Новое optional API field | `MINOR` | Старые клиенты продолжают работать |
| Новый config key с совместимым default | `MINOR` | Расширение конфигурации |
| Переименование config key с fallback и warning | `MINOR` | Старая форма временно поддерживается |
| Удаление старого config key после deprecation | `MAJOR` | Старые конфиги перестают работать |
| Изменение JSON-типа существующего API field | `MAJOR` | Ломает клиентов API v1 |
| Миграция DB с сохранением данных и rollback | `MINOR` | Совместимое развитие storage |
| Исправление auth bypass | `PATCH` | Security fix, ограничения описываются отдельно |
| Только ускорение cache/scheduler | `PATCH` | Наблюдаемый контракт не изменен |

## Правила оценки изменений

Перед merge пользовательского изменения автор отвечает на вопросы:

1. Меняется ли публичная поверхность из списка выше?
2. Продолжат ли существующие клиенты, конфиги и данные работать без ручного вмешательства?
3. Нужны ли deprecation, migration, rollback или security note?
4. Есть ли contract tests для старого и нового поведения?
5. В какую changelog category относится изменение?

Если ответ неоднозначен, изменение считается compatibility-sensitive и не получает меньшую версию только потому,
что реализация выглядит небольшой.

Внутренние изменения без пользовательского эффекта не требуют отдельной changelog-записи.

## Подготовка changelog к релизу

Maintainer выполняет следующие шаги до создания release tag:

1. Проверяет, что user-impacting изменения текущей линии перечислены в `CHANGELOG.md` под `Unreleased`, а known
   limitations не сформулированы как гарантии.
2. Переносит готовые записи в единственную секцию точной версии, например
   `## [1.0.0-beta.1] - 2026-07-15`, и создаёт новый шаблон `Unreleased`.
3. Не переносит `Нет.` из пустых категорий в release section, если категория не несёт пользовательской информации.
4. Проверяет сгенерированный текст локально:

```bash
./.github/scripts/extract-release-notes.sh CHANGELOG.md 1.0.0-beta.1 /tmp/release-notes.md
cat /tmp/release-notes.md
```

5. Запускает локальные release-проверки и только после их успешного завершения создает annotated или signed
   annotated tag.

Release workflow извлекает только секцию точной версии. Отсутствующая, дублированная, пустая или неверно датированная
секция останавливает публикацию. Текст из commit messages автоматически не используется.

## Локальная проверка beta-кандидата

Перед созданием beta-тега maintainer проверяет clean worktree и выполняет минимальный воспроизводимый gate:

```bash
make test-release
make test
make lint
make build-all
git diff --check
```

`make test-release` проверяет SemVer-теги, build metadata, извлечение release notes и сборку checksum manifest.
GitHub Actions повторно выполняет test, race и lint перед созданием release artifacts. Runtime performance и ручной
playback остаются отдельной локальной проверкой и не маскируются успешной сборкой.

На текущей private-beta стадии pipeline намеренно не публикует SBOM и provenance attestations и не требует
contributor/PR workflow или полного автоматического performance gate. Эти проверки возвращаются перед публичным
stable-релизом, когда их эксплуатационная ценность будет оправдывать стоимость сопровождения.

## Создание тегов

Release tag создается только на принятом commit основной ветки после соответствующего gate:

```bash
git tag -a v1.0.0-beta.1 -m "TorrServerV2 v1.0.0-beta.1"
git push origin v1.0.0-beta.1
```

При настроенной подписи используется signed annotated tag. Команда `git push --tags` не применяется: отправляется
только конкретный проверенный тег.

Для signed annotated tag при настроенной GPG/SSH-подписи используется:

```bash
git tag -s v1.0.0-beta.1 -m "TorrServerV2 v1.0.0-beta.1"
git push origin v1.0.0-beta.1
```

Release workflow принимает только перечисленные выше формы SemVer. Для воспроизводимости тег создается на
проверенном commit основной ветки и после публикации не перемещается.

## Восстановление после ошибки релиза

1. Если workflow упал до создания GitHub Release из-за временной ошибки runner или artifact upload, его можно
   перезапустить для того же неизмененного тега.
2. Если исправление требует нового commit, существующий tag не перемещается. Создается следующая версия, например
   `v1.0.0-beta.2`, и отправляется только этот новый tag.
3. Если GitHub Release уже создан, workflow намеренно запрещает повторную публикацию и замену assets. Исправление
   выпускается под новой версией; опубликованный tag и Release не удаляются ради повторного использования имени.
4. Перед повторным запуском проверяются Actions log, наличие Release и соответствие `git rev-list -n 1 <tag>`
   ожидаемому commit. Force-push tag и массовый `git push --tags` не используются.
