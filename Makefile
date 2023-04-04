GORELEASER_PARALLELISM ?= $(shell nproc --ignore=1)
GORELEASER_DEBUG ?= false
export DOCKERHUB_ORG ?= mhrabovcin
export GIT_TREE_STATE ?=

.PHONY: test
test:
	go test ./...

.PHONY: lint
lint:
	helm lint --strict ./charts/troubleshoot-live
	golangci-lint run --fix

ifndef GORELEASER_CURRENT_TAG
export GORELEASER_CURRENT_TAG=$(GIT_TAG)
endif

.PHONY: build-snapshot
build-snapshot:
	goreleaser --debug=$(GORELEASER_DEBUG) \
		build \
		--snapshot \
		--clean \
		--parallelism=$(GORELEASER_PARALLELISM) \
		$(if $(BUILD_ALL),,--single-target) \
		--skip-post-hooks

.PHONY: release
release:
	goreleaser --debug=$(GORELEASER_DEBUG) \
		release \
		--clean \
		--parallelism=$(GORELEASER_PARALLELISM) \
		--timeout=60m \
		$(GORELEASER_FLAGS)

.PHONY: release-snapshot
release-snapshot:
	goreleaser --debug=$(GORELEASER_DEBUG) \
		release \
		--snapshot \
		--skip-publish \
		--clean \
		--parallelism=$(GORELEASER_PARALLELISM) \
		--timeout=60m
