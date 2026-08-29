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

## dev: start the Go API and both Angular dev servers together.
dev:
	$(GO) run ./cmd/server & \
	$(PNPM) start:public & \
	$(PNPM) start:vendor-admin & \
	wait

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
