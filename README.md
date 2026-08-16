# Wget

A Go implementation of the 01-edu `wget` assignment. It supports streamed file downloads, progress reporting, custom output names and directories, rate limiting, background mode, concurrent batch downloads, and resilient concurrent website mirroring.

· [Русская версия](README_RU.md)

## 📋 TOC

- [🚀 Quick start](#-quick-start)
- [📝 About](#-about)
- [✨ Download features](#-download-features)
- [🪞 Website mirroring](#-website-mirroring)
- [⚡ Efficiency and resilience](#-efficiency-and-resilience)
- [🧪 Tests and verification](#-tests-and-verification)
- [📋 Official audit commands](#-official-audit-commands)
- [📁 Project structure](#-project-structure)
- [⚠️ Notes](#️-notes)
- [🧑‍💻 Authors](#-authors)

## 🚀 Quick start

### Requirements

- Go 1.20 or newer
- GNU Make for the one-command audit target
- no third-party Go packages

### Build

```bash
go build -o wget .
```

### Download a file

```bash
./wget https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

Normal output includes start/end time in `yyyy-mm-dd hh:mm:ss`, HTTP status, content size, destination path, downloaded KiB/MiB, percentage, speed and ETA.

### One-command verification

```bash
make audit
```

This is the recommended evaluator entry point.

## 📝 About

The project implements the required behavior directly in Go and does not wrap the system `wget`. Downloads are streamed to disk instead of buffering the complete file in memory.

HTTP redirects are followed. A non-`200 OK` response stops the affected ordinary download. Incomplete ordinary downloads and interrupted mirror resources are not left behind as completed-looking files.

For chunked responses where HTTP does not provide a content length, the program prints:

```text
content size: unknown
```

instead of exposing Go's internal `-1` value.

## ✨ Download features

### Output name — `-O`

```bash
./wget -O=test_20MB.zip https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

Both `-O=name` and `-O name` are accepted.

### Output directory — `-P`

```bash
./wget -O=test_20MB.zip -P=~/Downloads/ https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

`~` is expanded and missing directories are created.

With `--mirror`, `-P` is also supported and selects the parent directory for the mirrored host folder:

```bash
./wget --mirror -P=./mirrors https://example.com/
```

### Rate limit — `--rate-limit`

```bash
./wget --rate-limit=300k https://assets.01-edu.org/wgetDataSamples/20MB.zip
./wget --rate-limit=700k https://assets.01-edu.org/wgetDataSamples/20MB.zip
./wget --rate-limit=2M   https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

`k`/`K` and `M`/`m` suffixes are supported. Throttling is applied while bytes are read, not only corrected after completion.

The same limit can be combined with mirroring:

```bash
./wget --mirror --rate-limit=700k https://example.com/
```

The mirror uses one shared limiter, so the limit applies to the **aggregate traffic of all mirror workers**, not separately to each worker.

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

The files download concurrently. Batch mode deliberately suppresses per-download progress chatter. It prints the content-size list first (comma-separated, using `unknown` for chunked responses), then clean `finished <name>` lines and the final URL list.

### Background mode — `-B`

```bash
./wget -B https://assets.01-edu.org/wgetDataSamples/20MB.zip
```

The parent immediately prints:

```text
Output will be written to "wget-log".
```

The detached child continues the download. `wget-log` contains the audit-required start time, status, size, destination, completed URL and finish time without a progress-bar line or an extra blank separator.

## 🪞 Website mirroring

```bash
./wget --mirror https://example.com/
```

The mirror is stored under a folder named after the host. The assignment-required references are followed:

- `a[href]`
- `link[href]`
- `img[src]`

CSS resources are also discovered through:

- external `.css` files;
- `url(...)`;
- quoted `@import`;
- inline `<style>...</style>` blocks;
- inline `style="..."` attributes.

`script[src]` is intentionally **not** added to the crawl scope because the assignment explicitly names `a`, `link` and `img`. Script bodies are skipped by the HTML scanner, so strings such as `document.write("<img ...>")` do not create false resource downloads.

References using `data:`, `mailto:`, `javascript:` and `tel:` are ignored.

### Reject file types — `-R` / `--reject`

```bash
./wget --mirror -R=jpg,gif https://example.com/
./wget --mirror --reject=gif https://example.com/
```

Suffix matching is case-insensitive and happens before scheduling the request.

### Exclude directories — `-X` / `--exclude`

```bash
./wget --mirror -X=/assets,/img https://example.com/
```

Directory boundaries are respected: `/img` excludes `/img` and `/img/...`, but not `/images`.

### Convert links — `--convert-links`

```bash
./wget --mirror --convert-links https://example.com/
```

Downloaded HTML and CSS references are rewritten to local relative paths. External and filtered references remain unchanged.

Extensionless URLs are mapped using the response MIME type: for example, HTML `/about` becomes `about.html`, `image/png` `/logo` becomes `logo.png`, and JSON `/api/data` becomes `api/data.json`. This avoids file/directory collisions without pretending every extensionless resource is HTML. Query variants use a deterministic short hash in the local filename, so `/?page=1` and `/?page=2` cannot overwrite the same local file.

## ⚡ Efficiency and resilience

The mirror implementation also targets the audit bonus for speed and effective recursion:

- the root page is validated first;
- discovered resources are processed by a bounded worker pool (4 workers by default), balancing concurrency with polite request pressure on public sites;
- URL scheduling and the visited set are coordinated centrally, avoiding duplicate requests without a shared-map race;
- duplicate references are requested only once;
- child request, body-read and filesystem failures are logged as `skip ...` and do not abort the remaining mirror;
- root failures remain fatal, because there is no valid mirror without the requested root;
- files are written through temporary files and renamed only after a complete write, so interrupted resources do not look complete;
- same-site restrictions are preserved after redirects;
- when `--rate-limit` is used with `--mirror`, one shared limiter caps aggregate traffic across all workers.

The project remains standard-library-only.

## 🧪 Tests and verification

For the evaluator, run:

```bash
make audit
```

It performs:

```text
gofmt check
go vet ./...
go test ./... -count=1 -v
go test -race ./... -count=1
go build -o wget .
```

The black-box suite builds the real CLI binary and verifies, using deterministic local HTTP servers:

- normal download contents and required output fields;
- timestamp format;
- `-O` + `-P`;
- chunked/unknown content length;
- actual `300k` throttling by elapsed time;
- concurrent and readable `-i` behavior, including size-first comma-separated output and `unknown` chunked sizes;
- `-B` launcher output and exact `wget-log` shape;
- HTTP error cleanup;
- mirror `-P`;
- recursive mirror and offline link conversion;
- aggregate `--rate-limit` across concurrent mirror workers;
- MIME-aware local names for extensionless resources;
- inline CSS resources;
- required `a` / `link` / `img` scope without fetching scripts;
- `--reject=gif` and `-X=/img`;
- rejection of `-O` with mirror while allowing supported `-P` and `--rate-limit` combinations.

Lower-level mirror tests additionally prove:

- a broken child response does not abort healthy siblings;
- a partial broken resource is removed;
- extensionless page/resource directory collisions are avoided;
- query variants do not overwrite each other;
- duplicate URLs are fetched once;
- mirror downloads actually overlap in time;
- reject matching is case-insensitive and exclude matching respects directory boundaries;
- ASCII-only case folding preserves UTF-8 byte offsets while skipping `<script>`/`<style>` bodies.

Windows cross-compilation can be checked with:

```bash
GOOS=windows GOARCH=amd64 go build -o wget.exe .
```

## 📋 Official audit commands

`make audit` covers everything that can be tested deterministically. The official checklist also uses live third-party URLs, so run these before the evaluation if those sites are available.

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

# Supported together: aggregate limit across mirror workers
./wget --mirror --rate-limit=700k https://example.com/
```

The checklist also asks for one additional website of the evaluator's choice.

## 📁 Project structure

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

## ⚠️ Notes

- Mirror traversal stays on the requested host and also accepts the final host of the initial root redirect.
- `-R`/`--reject`, `-X`/`--exclude` and `--convert-links` require `--mirror`.
- `-i` and a positional URL are mutually exclusive.
- `-O` is rejected with `--mirror`; `-P` and `--rate-limit` are supported, with mirror rate limiting applied to aggregate worker traffic.
- Public audit sites can change or disappear independently of this repository; deterministic automated tests therefore do not depend on them.

## 🧑‍💻 Authors

- Nazar Yestayev (@nyestaye)
- Alexey Chen (@achen)
- Sultan Yersultan (@syersult)
- Aiman Zhumabayeva (@azhumaba)
