.PHONY: audit test vet fmt-check build clean

BINARY := wget

# One-command evaluator entry point: formatting, static analysis, the complete
# deterministic unit/black-box audit suite, and a final production build.
audit: fmt-check vet test build
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

vet:
	go vet ./...

build:
	go build -o $(BINARY) .

clean:
	rm -f $(BINARY) $(BINARY).exe wget-log
