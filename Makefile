SOURCE_FILES?=./...
TEST_PATTERN?=.
TEST_OPTIONS?=
TEST_TIMEOUT?=15m
TEST_PARALLEL?=2
DOCKER_BUILDKIT?=1
export DOCKER_BUILDKIT

export PATH := ./bin:$(PATH)
export GO111MODULE := on

# Install all the build and lint dependencies
setup:
	go mod tidy
	git config core.hooksPath .githooks
.PHONY: setup

pull_test_imgs:
	grep -m 1 FROM ./testdata/acceptance/*.dockerfile | cut -f2 -d' ' | sort | uniq | while read -r img; do docker pull "$$img"; done
.PHONY: pull_test_imgs

acceptance: pull_test_imgs
	make -e TEST_PARALLEL="4" -e TEST_OPTIONS="-tags=acceptance" -e TEST_TIMEOUT="60m" -e SOURCE_FILES="acceptance_test.go" test
.PHONY: acceptance

test:
	go test $(TEST_OPTIONS) -p $(TEST_PARALLEL) -v -failfast -race -coverpkg=./... -covermode=atomic -coverprofile=coverage.txt $(SOURCE_FILES) -run $(TEST_PATTERN) -timeout=$(TEST_TIMEOUT)
.PHONY: test

cover: test
	go tool cover -html=coverage.txt
.PHONY: cover

fmt:
	gofumpt -w .
.PHONY: fmt

lint: check
	golangci-lint run
.PHONY: check

ci: lint test
.PHONY: ci

build:
	go build -o nfpm ./cmd/nfpm/main.go
.PHONY: build

deps:
	go get -u
	go mod tidy
	go mod verify
.PHONY: deps

imgs:
	wget -O www/docs/static/logo.png https://github.com/goreleaser/artwork/raw/master/goreleaserfundo.png
	wget -O www/docs/static/card.png "https://og.caarlos0.dev/**NFPM**%20|%20A%20simple%20Deb%20and%20RPM%20packager%20written%20in%20Go.png?theme=light&md=1&fontSize=80px&images=https://github.com/goreleaser.png"
	wget -O www/docs/static/avatar.png https://github.com/goreleaser.png
	convert www/docs/static/avatar.png -define icon:auto-resize=64,48,32,16 www/docs/static/favicon.ico
	convert www/docs/static/avatar.png -resize x120 www/docs/static/apple-touch-icon.png
.PHONY: imgs

serve:
	@docker run --rm -it -p 8000:8000 -v ${PWD}/www:/docs squidfunk/mkdocs-material
.PHONY: serve

todo:
	@grep \
		--exclude-dir=vendor \
		--exclude-dir=node_modules \
		--exclude-dir=bin \
		--exclude=Makefile \
		--text \
		--color \
		-nRo -E ' TODO:.*|SkipNow' .
.PHONY: todo

.DEFAULT_GOAL := build
