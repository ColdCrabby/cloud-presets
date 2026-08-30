# Cold Crabby preset cloud — build targets for the Go API and both Angular apps.

GO ?= go
PNPM ?= pnpm
OPENAPI_OUT ?= openapi.yaml

.PHONY: build run dev test vet tidy openapi

## build: compile the Go API binary and build both Angular apps.
## The vendor app bakes in STYTCH_PUBLIC_TOKEN (if set) for real sign-in.
build:
	$(GO) build -o bin/server ./cmd/server
	$(PNPM) install --frozen-lockfile
	$(PNPM) build

## run: serve the Go API and both built frontends from one server (binds $PORT).
run:
	./bin/server

## dev: one unified dev origin (http://localhost:5200) mirroring the deployment.
## The Go server reverse-proxies the public app (/), vendor app (/vendor/) and
## the sample API (/v1) to their dev servers. See tools/dev.mjs.
dev:
	$(PNPM) dev

## test: run the test suite.
test:
	$(GO) test ./...

## vet: run go vet static checks.
vet:
	$(GO) vet ./...

## tidy: sync go.mod / go.sum.
tidy:
	$(GO) mod tidy

## openapi: export the OpenAPI 3.1 document consumed by the Angular client generator.
openapi:
	$(GO) run ./cmd/openapi $(OPENAPI_OUT)
