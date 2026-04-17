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
	DOCKER_COMPOSE="$(DOCKER_COMPOSE)" ./scripts/docker-up.sh

docker-down:
	$(DOCKER_COMPOSE) down --remove-orphans

smoke-local:
	./scripts/smoke-local.sh

smoke-docker:
	DOCKER_COMPOSE="$(DOCKER_COMPOSE)" ./scripts/smoke-docker.sh
