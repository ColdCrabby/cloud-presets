# Cold Crabby preset cloud — Go API build targets.

GO ?= go
OPENAPI_OUT ?= openapi.yaml

.PHONY: build run test vet tidy openapi

## build: compile all packages and commands.
build:
	$(GO) build ./...

## run: start the API server (ADDR overrides the listen address).
run:
	$(GO) run ./cmd/server

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
