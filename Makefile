.PHONY: audit test race vet fmt-check build clean

BINARY := wget

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
	go build -o $(BINARY) .

clean:
	rm -f $(BINARY) $(BINARY).exe wget-log
