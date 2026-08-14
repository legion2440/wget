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

For the evaluator, the shortest automated path is one command:

```bash
make audit
```

It checks formatting, runs `go vet`, executes the full unit and black-box test suite, and performs a final production build. The black-box tests compile the real `wget` binary and invoke it as a CLI against deterministic local HTTP servers, so they verify user-visible behavior rather than only internal functions.

Plain Go tests are also sufficient for the functional automated suite:

```bash
go test ./... -count=1 -v
```

The automated audit covers:

- normal file download and downloaded content;
- start/end timestamp format, HTTP status, content length, destination and 100% progress output;
- `-O` together with `-P`;
- actual `300k` rate throttling by elapsed transfer time;
- concurrent `-i` downloads using deliberately delayed endpoints;
- `-B`, detached completion and the required `wget-log` structure;
- non-`200 OK` handling without leaving a false completed file;
- recursive website mirroring;
- `--convert-links` with HTML, CSS, image and JavaScript resources;
- `--reject=gif`;
- `-X=/img`.

The lower-level tests additionally cover assignment CLI syntax, redirects, rate parsing for `300k`, `700k`, `2M` and `1.5M`, partial batch failures, exclusion boundaries and HTML/CSS link handling.

Useful individual checks:

```bash
go test -race ./...
go vet ./...
go build -o wget .
```

Windows cross-compilation can be checked with:

```bash
GOOS=windows GOARCH=amd64 go build -o wget.exe .
```

## 📋 Audit commands

`make audit` automates everything that can be tested deterministically without relying on third-party websites. The official checklist also asks the evaluator to try live public URLs; those remain manual because the sites can disappear or change independently of this project.

Build once for the live checks:

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
├── Makefile
├── README.md
├── README_RU.md
├── audit_test.go
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
