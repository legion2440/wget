# Wget

A Go implementation of the 01-edu `wget` assignment. It supports file downloads with progress reporting, custom names and directories, rate limiting, background mode, concurrent batch downloads, and recursive website mirroring.

· [Русская версия](README_RU.md)

## 📋 TOC

- [🚀 Quick start](#-quick-start)
- [📝 About](#-about)
- [✨ Features](#-features)
- [🪞 Website mirroring](#-website-mirroring)
- [🧪 Tests and verification](#-tests-and-verification)
- [📋 Audit commands](#-audit-commands)
- [📁 Project structure](#-project-structure)
- [⚠️ Notes](#️-notes)
- [🧑‍💻 Authors](#-authors)

## 🚀 Quick start

### Requirements

- Go 1.20 or newer
- no third-party packages

### Build

```bash
go build -o wget .
```

### Download a file

```bash
./wget https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

The normal download output includes the start and finish time (`yyyy-mm-dd hh:mm:ss`), HTTP status, raw and rounded content size, destination path, downloaded KiB/MiB, percentage, current speed and ETA.

## 📝 About

The project implements the required behavior directly in Go and does not wrap the system `wget` command. Ordinary downloads are streamed to disk, so the complete payload is not kept in memory.

HTTP redirects are followed automatically. A non-`200 OK` response stops the affected download, and an incomplete ordinary download is removed instead of being left as a completed-looking file.

## ✨ Features

### Output name — `-O`

```bash
./wget -O=test_20MB.zip https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

Both `-O=name` and `-O name` forms are accepted.

### Output directory — `-P`

```bash
./wget -O=test_20MB.zip -P=~/Downloads/ https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

`~` is expanded to the current user's home directory. Missing destination directories are created automatically.

### Rate limit — `--rate-limit`

```bash
./wget --rate-limit=300k https://assets.01-edu.org/wgetDataSamples/20MB.zip
./wget --rate-limit=700k https://assets.01-edu.org/wgetDataSamples/20MB.zip
./wget --rate-limit=2M   https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

`k`/`K` and `M`/`m` suffixes are supported. The reader is paced while bytes are returned, so transfer time is constrained by the requested limit instead of being corrected only after completion.

### Concurrent input file — `-i`

Create `downloads.txt`:

```text
https://assets.01-edu.org/wgetDataSamples/Image_20MB.zip
https://assets.01-edu.org/wgetDataSamples/20MB.zip
https://assets.01-edu.org/wgetDataSamples/Image_10MB.zip
```

Run:

```bash
./wget -i=downloads.txt
```

Each URL is downloaded concurrently in its own goroutine. Empty lines and lines beginning with `#` are ignored. A failed URL is reported without cancelling the other downloads.

### Background mode — `-B`

```bash
./wget -B https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

The parent process immediately prints:

```text
Output will be written to "wget-log".
```

The detached child continues the download and writes its status output to `wget-log`. The progress bar is disabled in the log so its structure remains readable and audit-friendly.

## 🪞 Website mirroring

```bash
./wget --mirror https://example.com/
```

A mirror is stored in a directory named after the site host. The crawler recursively follows same-site references, tracks visited URLs to avoid cycles, and reproduces URL paths in the local filesystem.

The required HTML references are supported:

- `a[href]`
- `link[href]`
- `img[src]`

`script[src]` is also downloaded to improve offline behavior. CSS `url(...)` and quoted `@import` references are followed as well.

`data:`, `mailto:`, `javascript:` and `tel:` references are ignored.

### Reject file types — `-R` / `--reject`

```bash
./wget --mirror -R=jpg,gif https://example.com/
./wget --mirror --reject=gif https://example.com/
```

Suffix matching is case-insensitive and happens before the resource request.

### Exclude directories — `-X` / `--exclude`

```bash
./wget --mirror -X=/assets,/img https://example.com/
```

Directory boundaries are respected: `/img` excludes `/img` and `/img/...`, but not `/images`.

### Offline links — `--convert-links`

```bash
./wget --mirror --convert-links https://example.com/
```

References in downloaded HTML and CSS are converted to relative local paths. External and filtered references remain unchanged.

## 🧪 Tests and verification

Run all automated tests:

```bash
go test ./... -v
```

Additional checks:

```bash
go test -race ./...
go vet ./...
gofmt -l main.go internal
go build -o wget .
```

`gofmt -l` should print nothing.

The tests use deterministic local `httptest` servers and cover:

- assignment CLI syntax;
- ordinary downloads and `-O`;
- redirects and HTTP failures;
- rate parsing and pacing;
- concurrent `-i` downloads;
- keeping successful batch downloads when one URL fails;
- recursive mirror traversal;
- `--reject` and `--exclude`;
- exclusion directory boundaries;
- HTML and CSS link conversion.

The background implementation is split by Go build tags. Windows cross-compilation can be checked with:

```bash
GOOS=windows GOARCH=amd64 go build -o wget.exe .
```

## 📋 Audit commands

Build once:

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

The audit also asks the evaluator to mirror one additional site of their choice.

## 📁 Project structure

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

## ⚠️ Notes

- Mirror traversal stays on the requested host and also accepts the final host of the initial HTTP redirect.
- Query strings are part of URL deduplication; local storage follows the URL path.
- `-R`/`--reject`, `-X`/`--exclude` and `--convert-links` require `--mirror`.
- `-i` and a positional URL are mutually exclusive.
- Public audit sites may change independently of this repository, so automated tests do not depend on them.

## 🧑‍💻 Authors

- Nazar Yestayev (@nyestaye)
- Alexey Chen (@achen)
- Sultan Yersultan (@syersult)
- Aiman Zhumabayeva (@azhumaba)
