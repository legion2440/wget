# Wget

Реализация задания 01-edu `wget` на Go. Программа поддерживает потоковое скачивание файлов, progress bar, изменение имени и каталога, ограничение скорости, фоновый режим, параллельные загрузки из списка и устойчивое параллельное зеркалирование сайтов.

· [English version](README.md)

## 📋 Содержание

- [🚀 Быстрый старт](#-быстрый-старт)
- [📝 О проекте](#-о-проекте)
- [✨ Возможности загрузки](#-возможности-загрузки)
- [🪞 Зеркалирование сайтов](#-зеркалирование-сайтов)
- [⚡ Эффективность и устойчивость](#-эффективность-и-устойчивость)
- [🧪 Тесты и проверка](#-тесты-и-проверка)
- [📋 Команды официального аудита](#-команды-официального-аудита)
- [📁 Структура проекта](#-структура-проекта)
- [⚠️ Примечания](#️-примечания)
- [🧑‍💻 Авторы](#-авторы)

## 🚀 Быстрый старт

### Требования

- Go 1.20 или новее
- GNU Make для проверки одной командой
- сторонние Go-пакеты не используются

### Сборка

```bash
go build -o wget .
```

### Скачивание файла

```bash
./wget https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

Обычный вывод содержит время начала и окончания в формате `yyyy-mm-dd hh:mm:ss`, HTTP status, размер, путь сохранения, KiB/MiB, процент, скорость и ETA.

### Проверка одной командой

```bash
make audit
```

Это основной вход для аудитора.

## 📝 О проекте

Проект самостоятельно реализует требуемое поведение и не вызывает системный `wget`. Файлы записываются на диск потоково без загрузки целого payload в память.

HTTP redirects поддерживаются. Ответ, отличный от `200 OK`, останавливает соответствующую обычную загрузку. Незавершённые файлы и оборванные mirror-ресурсы не остаются на диске как готовые.

Для chunked-ответов без известного Content-Length выводится:

```text
content size: unknown
```

вместо внутреннего значения Go `-1`.

## ✨ Возможности загрузки

### Имя файла — `-O`

```bash
./wget -O=test_20MB.zip https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

Поддерживаются `-O=name` и `-O name`.

### Каталог — `-P`

```bash
./wget -O=test_20MB.zip -P=~/Downloads/ https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

`~` раскрывается, недостающие каталоги создаются автоматически. Для `--mirror` флаг `-P` задаёт родительский каталог зеркала:

```bash
./wget --mirror -P=./mirrors https://example.com/
```

### Ограничение скорости — `--rate-limit`

```bash
./wget --rate-limit=300k https://assets.01-edu.org/wgetDataSamples/20MB.zip
./wget --rate-limit=700k https://assets.01-edu.org/wgetDataSamples/20MB.zip
./wget --rate-limit=2M   https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

Поддерживаются `k`/`K` и `M`/`m`. Ограничение применяется во время чтения потока.

Флаг работает и вместе с mirror:

```bash
./wget --mirror --rate-limit=700k https://example.com/
```

Для mirror используется один общий limiter: указанная скорость ограничивает **суммарный трафик всех worker'ов**, а не выдаётся каждому worker'у отдельно.

### Параллельные загрузки — `-i`

`downloads.txt`:

```text
https://assets.01-edu.org/wgetDataSamples/Image_20MB.zip
https://assets.01-edu.org/wgetDataSamples/20MB.zip
https://assets.01-edu.org/wgetDataSamples/Image_10MB.zip
```

Запуск:

```bash
./wget -i=downloads.txt
```

Файлы скачиваются одновременно. Batch-режим глушит подробный вывод отдельных downloader'ов. Сначала печатается список размеров через запятую (`unknown` для chunked), затем строки `finished <name>` и итоговый список URL.

### Фоновый режим — `-B`

```bash
./wget -B https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

Родитель сразу выводит:

```text
Output will be written to "wget-log".
```

Detached child продолжает работу. В `wget-log` остаётся структура из audit-листа без progress bar и лишней пустой строки перед `Downloaded`.

## 🪞 Зеркалирование сайтов

```bash
./wget --mirror https://example.com/
```

Зеркало сохраняется в каталоге с именем хоста. Обрабатываются обязательные по заданию ссылки:

- `a[href]`
- `link[href]`
- `img[src]`

CSS-ресурсы находятся через внешние `.css`, `url(...)`, строковый `@import`, inline `<style>...</style>` и атрибуты `style="..."`.

`script[src]` намеренно не добавлен в crawl scope: задание явно перечисляет `a`, `link`, `img`. Тело `<script>` сканер пропускает целиком, поэтому строка вроде `document.write("<img ...>")` не создаёт ложную загрузку.

`data:`, `mailto:`, `javascript:` и `tel:` игнорируются.

### Исключение типов — `-R` / `--reject`

```bash
./wget --mirror -R=jpg,gif https://example.com/
./wget --mirror --reject=gif https://example.com/
```

Расширения сравниваются без учёта регистра до постановки URL в очередь.

### Исключение каталогов — `-X` / `--exclude`

```bash
./wget --mirror -X=/assets,/img https://example.com/
```

`/img` исключает `/img` и `/img/...`, но не `/images`.

### Офлайн-ссылки — `--convert-links`

```bash
./wget --mirror --convert-links https://example.com/
```

HTML/CSS-ссылки переписываются в относительные локальные пути после того, как фактические ресурсы скачаны и известны их локальные имена. Внешние и отфильтрованные ссылки не изменяются.

URL без расширения маппятся по MIME-типу ответа: HTML `/about` становится `about.html`, `image/png` `/logo` — `logo.png`, JSON `/api/data` — `api/data.json`. Это убирает file/directory collision и не приписывает `.html` бинарным ресурсам. Query-варианты получают короткий детерминированный hash в имени файла и не затирают друг друга.

## ⚡ Эффективность и устойчивость

Mirror закрывает и бонусный пункт по эффективности:

- root проверяется первым;
- ресурсы обрабатывает ограниченный worker pool — по умолчанию 4 worker'а;
- очередь и visited set управляются одним coordinator'ом, поэтому нет shared-map race;
- один URL запрашивается только один раз, даже если встречается многократно;
- ошибки request/read/filesystem у дочернего ресурса логируются как `skip ...` и не останавливают остальные загрузки;
- ошибка root остаётся fatal;
- запись идёт через временный файл с rename только после полного сохранения;
- same-site ограничение сохраняется после redirects;
- `--mirror --rate-limit` использует один общий limiter для суммарного трафика worker'ов.

Проект остаётся stdlib-only.

## 🧪 Тесты и проверка

Для аудитора:

```bash
make audit
```

Команда выполняет:

```text
gofmt check
go vet ./...
go test ./... -count=1 -v
go test -race ./... -count=1
go build -o wget .
```

Black-box тесты собирают настоящий CLI и проверяют:

- обычную загрузку и обязательный вывод;
- формат времени;
- `-O` + `-P`;
- chunked/unknown Content-Length;
- фактический rate limit;
- конкурентный `-i` и его size-first вывод;
- `-B` и структуру `wget-log`;
- очистку после HTTP error;
- `-P` для mirror;
- рекурсивный mirror и `--convert-links`;
- общий `--rate-limit` для параллельных mirror worker'ов;
- MIME-aware имена extensionless ресурсов;
- inline CSS;
- обязательные `a` / `link` / `img`, без загрузки scripts;
- `--reject=gif` и `-X=/img`;
- запрет `-O` с mirror.

Нижнеуровневые тесты дополнительно проверяют broken child responses, удаление partial-файлов, query collisions, duplicate URL, параллельность mirror, case-insensitive reject, границы exclude и сохранение UTF-8 byte offsets при поиске `</script>`/`</style>`.

Windows cross-build:

```bash
GOOS=windows GOARCH=amd64 go build -o wget.exe .
```

## 📋 Команды официального аудита

`make audit` автоматически закрывает детерминированные проверки. Перед сдачей остаётся прогнать живые URL из официального checklist.

### Functional

```bash
./wget https://pbs.twimg.com/media/EMtmPFLWkAA8CIS.jpg
./wget https://golang.org/dl/go1.16.3.linux-amd64.tar.gz
./wget https://assets.01-edu.org/wgetDataSamples/Sample.zip

./wget -O=test_20MB.zip https://assets.01-edu.org/wgetDataSamples/20MB.zip
./wget -O=test_20MB.zip -P=~/Downloads/ https://assets.01-edu.org/wgetDataSamples/20MB.zip

./wget --rate-limit=300k https://assets.01-edu.org/wgetDataSamples/20MB.zip
./wget --rate-limit=700k https://assets.01-edu.org/wgetDataSamples/20MB.zip
./wget --rate-limit=2M https://assets.01-edu.org/wgetDataSamples/20MB.zip

./wget -i=downloads.txt
./wget -B https://assets.01-edu.org/wgetDataSamples/20MB.zip
cat wget-log
```

### Mirror

```bash
./wget --mirror --convert-links http://corndog.io/
./wget --mirror https://oct82.com/
./wget --mirror --reject=gif https://oct82.com/
./wget --mirror https://trypap.com/
./wget --mirror -X=/img https://trypap.com/
./wget --mirror https://theuselessweb.com/

# Поддерживаемая комбинация: общий лимит на mirror workers
./wget --mirror --rate-limit=700k https://example.com/
```

Checklist также просит зеркалировать ещё один сайт на выбор аудитора.

## 📁 Структура проекта

```text
wget/
├── internal/
│   ├── background/
│   ├── cli/
│   ├── download/
│   └── mirror/
│       ├── css.go
│       ├── html.go
│       ├── mirror.go
│       ├── mirror_test.go
│       ├── path.go
│       └── traversal_test.go
├── .gitignore
├── Makefile
├── README.md
├── README_RU.md
├── audit_test.go
├── go.mod
└── main.go
```

## ⚠️ Примечания

- Mirror остаётся на исходном хосте и принимает конечный хост первого root redirect.
- `-R`/`--reject`, `-X`/`--exclude` и `--convert-links` требуют `--mirror`.
- `-i` и позиционный URL взаимоисключающие.
- `-O` с `--mirror` возвращает ошибку; `-P` и `--rate-limit` поддерживаются.
- Публичные сайты из аудита могут измениться независимо от проекта, поэтому автоматические тесты от них не зависят.

## 🧑‍💻 Авторы

- Nazar Yestayev (@nyestaye)
- Alexey Chen (@achen)
- Sultan Yersultan (@syersult)
- Aiman Zhumabayeva (@azhumaba)
