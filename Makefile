# Cold Crabby preset cloud — build targets for the Go API and both Angular apps.

GO ?= go
PNPM ?= pnpm
OPENAPI_OUT ?= openapi.yaml

.PHONY: build run test vet tidy openapi

## build: compile the Go API and build both Angular apps.
build:
	$(GO) build ./...
	$(PNPM) install --frozen-lockfile
	$(PNPM) build

## run: start the Go API and both Angular dev servers together.
run:
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
