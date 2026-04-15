GO ?= go

.PHONY: test fmt

test:
	$(GO) test ./...

fmt:
	gofmt -w $(shell find . -name '*.go' -not -path './.git/*')
