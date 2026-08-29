GO ?= go
OPENAPI_OUT ?= openapi.yaml

.PHONY: build run test vet tidy openapi

## build: compile all packages and commands
build:
	$(GO) build ./...

## run: start the API server
run:
	$(GO) run ./cmd/api

## test: run the test suite
test:
	$(GO) test ./...

## vet: run go vet
vet:
	$(GO) vet ./...

## tidy: sync go.mod / go.sum
tidy:
	$(GO) mod tidy

## openapi: export the OpenAPI 3.1 document to $(OPENAPI_OUT)
openapi:
	$(GO) run ./cmd/openapi $(OPENAPI_OUT)
