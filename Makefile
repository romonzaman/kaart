BINARY  := bin/kaartd
PKG     := ./...
DB      ?= ./kaart.db
ADDR    ?= 127.0.0.1:8080

.PHONY: all deps build test test-v lint fmt vet run migrate clean \
        app-deps app app-test app-typecheck app-check check

all: build

## deps: resolve and tidy module dependencies (run this first on a fresh clone)
deps:
	go mod tidy

## build: compile the server into bin/kaartd
build:
	go build -o $(BINARY) ./cmd/kaartd

## test: run the full test suite
test:
	go test $(PKG)

test-v:
	go test -v -count=1 $(PKG)

## lint: golangci-lint (install: https://golangci-lint.run/welcome/install/)
lint:
	golangci-lint run

fmt:
	gofmt -s -w .

vet:
	go vet $(PKG)

## run: start the server against $(DB) on $(ADDR)
run:
	go run ./cmd/kaartd --db $(DB) --addr $(ADDR)

## migrate: apply migrations to $(DB) and exit
migrate:
	go run ./cmd/kaartd --db $(DB) --migrate-only

clean:
	rm -rf bin $(DB) $(DB)-wal $(DB)-shm

# --- frontend ---

## app-deps: install the Expo app's dependencies
app-deps:
	cd app && npm install

## app: start the Expo dev server on the web (needs `make run` in another shell)
app:
	cd app && npx expo start --web

## app-typecheck: tsc --noEmit
app-typecheck:
	cd app && npx tsc --noEmit

## app-test: the frontend unit tests
app-test:
	cd app && npx jest --ci

app-check: app-typecheck app-test

## check: everything CI would run
check: vet test app-check
