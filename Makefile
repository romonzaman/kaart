BINARY  := bin/kaartd
PKG     := ./...
DB      ?= ./kaart.db
DIST    := dist
# URL prefix the app is served under. '/' for a domain root.
BASE_PATH ?= /kaartd
# The same prefix as a directory name, because nginx matches it with root.
BASE_DIR  := $(patsubst /%,%,$(BASE_PATH))
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
ADDR    ?= 127.0.0.1:8080

.PHONY: all deps build test test-v lint fmt vet run migrate clean \
        app-deps app app-test app-typecheck app-check check \
        build-linux web dist

all: build

## deps: resolve and tidy module dependencies (run this first on a fresh clone)
deps:
	go mod tidy

## build: compile the server into bin/kaartd
build:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY) ./cmd/kaartd

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
	rm -rf bin $(DIST) $(DB) $(DB)-wal $(DB)-shm

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

# --- deployment (see docs/deployment.md) ---

## build-linux: cross-compile a stripped server binary for a Debian 12 host
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
	  -trimpath -ldflags "-s -w -X main.version=$(VERSION)" \
	  -o $(DIST)/kaartd ./cmd/kaartd

## web: export the Expo app as static files for serving under $(BASE_PATH)
##
## EXPO_BASE_URL prefixes the asset URLs in index.html and expo-router's route
## matching. EXPO_PUBLIC_API_URL prefixes the client's request paths, so they
## come out as $(BASE_PATH)/api/v1/... — relative, same-origin, no CORS.
## Leave both unset and the bundle loads assets from / and calls
## http://localhost:8080, which is right for development and wrong on a server.
web:
	# A stale export from a different BASE_PATH would otherwise be shipped
	# alongside the new one and served at the wrong prefix.
	rm -rf $(DIST)/web
	cd app && EXPO_BASE_URL=$(BASE_PATH) EXPO_PUBLIC_API_URL=$(BASE_PATH) \
	  npx expo export --platform web --output-dir ../$(DIST)/web/$(BASE_DIR)

## dist: binary, web bundle and deploy files, ready to scp to the server
dist: build-linux web
	mkdir -p $(DIST)/deploy
	cp deploy/kaartd.service deploy/kaart.env.example \
	   deploy/nginx-kaartd.conf deploy/install.sh $(DIST)/deploy/
	chmod +x $(DIST)/deploy/install.sh
	@echo
	@echo "$(DIST)/ is ready. Deploy with:"
	@echo "  rsync -a --delete $(DIST)/ user@host:/tmp/kaart-dist/"
	@echo "  ssh user@host 'cd /tmp/kaart-dist && sudo ./deploy/install.sh'"
