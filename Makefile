.PHONY: audit test race vet fmt-check build clean

BINARY := wget
ifeq ($(OS),Windows_NT)
BINARY := wget-bin.exe
endif

audit: fmt-check vet test race build
	@echo "audit: all automated checks passed"

fmt-check:
	@files="$$(find . -name '*.go' -type f -not -path './.git/*')"; \
	bad="$$(gofmt -l $$files)"; \
	if [ -n "$$bad" ]; then \
		echo "gofmt required for:"; \
		echo "$$bad"; \
		exit 1; \
	fi

test:
	go test ./... -count=1 -v

race:
	go test -race ./... -count=1

vet:
	go vet ./...

build:
	rm -f wget wget.exe wget-bin.exe
	go build -o $(BINARY) .
ifeq ($(OS),Windows_NT)
	@printf '%s\n' '#!/usr/bin/env sh' 'MSYS2_ARG_CONV_EXCL="-X=;--exclude=" exec "$$(dirname "$$0")/wget-bin.exe" "$$@"' > wget
	@chmod +x wget
endif

clean:
	rm -f wget wget.exe wget-bin.exe wget-log
