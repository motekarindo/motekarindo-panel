GOCACHE ?= $(CURDIR)/.cache/go-build
GOENV := GOCACHE=$(GOCACHE)

.PHONY: build test test-system fmt fmt-check dev

build:
	$(GOENV) go build ./cmd/motekar-panel ./cmd/motekar-agent ./cmd/motekarctl

test:
	$(GOENV) go test ./...

test-system:
	@if [ "$(MOTEKAR_TEST_ALLOW_SYSTEM)" != "1" ]; then \
		echo "Refusing to run system tests on an unmarked environment."; \
		echo "Use a disposable Ubuntu 24.04 VM/container and set MOTEKAR_TEST_ALLOW_SYSTEM=1."; \
		exit 1; \
	fi
	$(GOENV) go test -tags=system ./tests/system/...

fmt:
	$(GOENV) go fmt ./...

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Run make fmt before committing."; gofmt -l .; exit 1)

dev:
	$(GOENV) go run ./cmd/motekar-panel serve
