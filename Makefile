GOCACHE ?= $(CURDIR)/.cache/go-build
GOENV := GOCACHE=$(GOCACHE)

.PHONY: build release-artifacts test test-integration-postgres test-installers test-release-artifacts test-system fmt fmt-check dev

build:
	$(GOENV) go build ./cmd/motekar-panel ./cmd/motekar-agent ./cmd/motekarctl

release-artifacts:
	$(GOENV) scripts/build-release.sh

test:
	$(GOENV) go test ./...

test-integration-postgres:
	@test -n "$(MOTEKAR_TEST_DATABASE_URL)" || (echo "MOTEKAR_TEST_DATABASE_URL is required."; exit 1)
	@test "$(MOTEKAR_TEST_ALLOW_DATABASE)" = "1" || (echo "MOTEKAR_TEST_ALLOW_DATABASE=1 is required."; exit 1)
	$(GOENV) go test -count=1 ./tests/integration/...

test-installers:
	tests/installers/test-installers.sh

test-release-artifacts:
	tests/release/test-release-artifacts.sh

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
