GO ?= go
DOCKER_COMPOSE ?= docker compose

.PHONY: test fmt docker-build docker-up docker-down smoke-local smoke-docker

test:
	$(GO) test ./...

fmt:
	gofmt -w $(shell find . -name '*.go' -not -path './.git/*')

docker-build:
	$(DOCKER_COMPOSE) build

docker-up:
	./scripts/docker-up.sh $(DOCKER_COMPOSE)

docker-down:
	$(DOCKER_COMPOSE) down --remove-orphans

smoke-local:
	./scripts/smoke-local.sh

smoke-docker:
	./scripts/smoke-docker.sh
