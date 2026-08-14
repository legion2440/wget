# Wget

Реализация задания 01-edu `wget` на Go. Программа поддерживает скачивание файлов с отображением прогресса, изменение имени и каталога, ограничение скорости, фоновый режим, параллельные загрузки из списка и рекурсивное зеркалирование сайтов.

· [English version](README.md)

## 📋 Содержание

- [🚀 Быстрый старт](#-быстрый-старт)
- [📝 О проекте](#-о-проекте)
- [✨ Возможности](#-возможности)
- [🪞 Зеркалирование сайтов](#-зеркалирование-сайтов)
- [🧪 Тесты и проверка](#-тесты-и-проверка)
- [📋 Команды аудита](#-команды-аудита)
- [📁 Структура проекта](#-структура-проекта)
- [⚠️ Примечания](#️-примечания)
- [🧑‍💻 Авторы](#-авторы)

## 🚀 Быстрый старт

### Требования

- Go 1.20 или новее
- сторонние пакеты не используются

### Сборка

```bash
go build -o wget .
```

### Скачивание файла

```bash
./wget https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

При обычной загрузке выводятся время начала и окончания (`yyyy-mm-dd hh:mm:ss`), HTTP-статус, размер в байтах и округлённом виде, путь сохранения, прогресс в KiB/MiB, процент, текущая скорость и оставшееся время.

## 📝 О проекте

Проект самостоятельно реализует требуемое поведением задания и не вызывает системную команду `wget`. Обычные загрузки записываются на диск потоково, поэтому полный файл не хранится в памяти.

HTTP redirect обрабатывается автоматически. Ответ, отличный от `200 OK`, останавливает соответствующую загрузку. Незавершённый обычный файл удаляется, чтобы на диске не оставался ложный готовый результат.

## ✨ Возможности

### Имя файла — `-O`

```bash
./wget -O=test_20MB.zip https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

Поддерживаются формы `-O=name` и `-O name`.

### Каталог — `-P`

```bash
./wget -O=test_20MB.zip -P=~/Downloads/ https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

`~` раскрывается в домашний каталог текущего пользователя. Недостающие каталоги создаются автоматически.

### Ограничение скорости — `--rate-limit`

```bash
./wget --rate-limit=300k https://assets.01-edu.org/wgetDataSamples/20MB.zip
./wget --rate-limit=700k https://assets.01-edu.org/wgetDataSamples/20MB.zip
./wget --rate-limit=2M   https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

Поддерживаются суффиксы `k`/`K` и `M`/`m`. Скорость ограничивается непосредственно во время чтения потока.

### Параллельные загрузки — `-i`

Создайте `downloads.txt`:

```text
https://assets.01-edu.org/wgetDataSamples/Image_20MB.zip
https://assets.01-edu.org/wgetDataSamples/20MB.zip
https://assets.01-edu.org/wgetDataSamples/Image_10MB.zip
```

Запуск:

```bash
./wget -i=downloads.txt
```

Каждый URL запускается параллельно в отдельной goroutine. Пустые строки и строки, начинающиеся с `#`, игнорируются. Ошибка одного URL не отменяет остальные загрузки.

### Фоновый режим — `-B`

```bash
./wget -B https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

Родительский процесс сразу выводит:

```text
Output will be written to "wget-log".
```

Отделённый дочерний процесс продолжает загрузку и пишет статус в `wget-log`. Progress bar в лог намеренно не выводится.

## 🪞 Зеркалирование сайтов

```bash
./wget --mirror https://example.com/
```

Зеркало сохраняется в каталог с именем хоста. Crawler рекурсивно проходит ссылки того же сайта, запоминает посещённые URL и воспроизводит URL-пути в локальной файловой системе.

Поддерживаются обязательные ссылки:

- `a[href]`
- `link[href]`
- `img[src]`

Дополнительно скачивается `script[src]`. В CSS обрабатываются `url(...)` и строковые `@import`.

Ссылки `data:`, `mailto:`, `javascript:` и `tel:` игнорируются.

### Исключение типов — `-R` / `--reject`

```bash
./wget --mirror -R=jpg,gif https://example.com/
./wget --mirror --reject=gif https://example.com/
```

Расширения сравниваются без учёта регистра до сетевого запроса.

### Исключение каталогов — `-X` / `--exclude`

```bash
./wget --mirror -X=/assets,/img https://example.com/
```

Границы каталогов учитываются: `/img` исключает `/img` и `/img/...`, но не `/images`.

### Офлайн-ссылки — `--convert-links`

```bash
./wget --mirror --convert-links https://example.com/
```

Ссылки в скачанных HTML и CSS преобразуются в относительные локальные пути. Внешние и отфильтрованные ссылки остаются без изменений.

## 🧪 Тесты и проверка

Полный набор тестов:

```bash
go test ./... -v
```

Дополнительные проверки:

```bash
go test -race ./...
go vet ./...
gofmt -l main.go internal
go build -o wget .
```

`gofmt -l` не должен ничего выводить.

Тесты используют локальные `httptest`-серверы и покрывают:

- синтаксис CLI из задания;
- обычную загрузку и `-O`;
- redirects и HTTP-ошибки;
- разбор и фактическое ограничение скорости;
- параллельный `-i`;
- сохранение успешных batch-загрузок при ошибке одного URL;
- рекурсивный mirror;
- `--reject` и `--exclude`;
- границы исключаемых каталогов;
- преобразование HTML/CSS-ссылок.

Фоновая реализация разделена build tags для Unix и Windows. Кросс-компиляция Windows:

```bash
GOOS=windows GOARCH=amd64 go build -o wget.exe .
```

## 📋 Команды аудита

Сборка:

```bash
go build -o wget .
```

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
```

Также аудитор выбирает ещё один произвольный сайт.

## 📁 Структура проекта

```text
wget/
├── internal/
│   ├── background/
│   │   ├── background.go
│   │   ├── background_unix.go
│   │   └── background_windows.go
│   ├── cli/
│   │   ├── options.go
│   │   └── options_test.go
│   ├── download/
│   │   ├── batch.go
│   │   ├── batch_test.go
│   │   ├── downloader.go
│   │   ├── downloader_test.go
│   │   ├── progress.go
│   │   └── ratelimit.go
│   └── mirror/
│       ├── css.go
│       ├── html.go
│       ├── mirror.go
│       ├── mirror_test.go
│       └── path.go
├── .gitignore
├── README.md
├── README_RU.md
├── go.mod
└── main.go
```

## ⚠️ Примечания

- Mirror остаётся на исходном хосте и также принимает конечный хост первого HTTP redirect.
- Query string участвует в дедупликации URL, локальный путь определяется URL-путём.
- `-R`/`--reject`, `-X`/`--exclude` и `--convert-links` требуют `--mirror`.
- `-i` и позиционный URL взаимоисключающие.
- Публичные сайты из аудита могут меняться независимо от проекта, поэтому автоматические тесты от них не зависят.

## 🧑‍💻 Авторы

- Nazar Yestayev (@nyestaye)
- Alexey Chen (@achen)
- Sultan Yersultan (@syersult)
- Aiman Zhumabayeva (@azhumaba)
