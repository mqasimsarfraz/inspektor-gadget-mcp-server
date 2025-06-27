GOHOSTOS ?= $(shell go env GOHOSTOS)
GOHOSTARCH ?= $(shell go env GOHOSTARCH)

TAG := `git describe --tags --always`
VERSION :=

LINTER_VERSION ?= v2.1.6

# Adds a '-dirty' suffix to version string if there are uncommitted changes
changes := $(shell git status --porcelain)
ifeq ($(changes),)
	VERSION := $(TAG)
else
	VERSION := $(TAG)-dirty
endif

LDFLAGS := "-X main.version=$(VERSION) -extldflags '-static'"

CONTAINER_REPO_NAMESPACE ?= ghcr.io/inspektor-gadget
CONTAINER_REPO_NAME ?= ig-mcp-server
IMAGE_TAG ?= latest
GADGET_IMAGES ?= trace_dns:latest,snapshot_process:latest,top_tcp:latest

IG_MCP_SERVER_TARGET = \
	ig-mcp-server-linux-amd64 \
	ig-mcp-server-linux-arm64 \
	ig-mcp-server-darwin-amd64 \
	ig-mcp-server-darwin-arm64 \
	ig-mcp-server-windows-amd64

.DEFAULT_GOAL := ig-mcp-server

# make does not allow implicit rules (with '%') to be phony so let's use
# the 'phony_explicit' dependency to make implicit rules inherit the phony
# attribute
.PHONY: phony_explicit
phony_explicit:

.PHONY: list-ig-mcp-server-targets
list-ig-mcp-server-targets:
	@echo $(IG_MCP_SERVER_TARGET)

.PHONY: ig-mcp-server-all
ig-mcp-server-all: $(IG_MCP_SERVER_TARGET) ig-mcp-server

ig-mcp-server: ig-mcp-server-$(GOHOSTOS)-$(GOHOSTARCH)
	cp ig-mcp-server-$(GOHOSTOS)-$(GOHOSTARCH)$(if $(findstring windows,$*),.exe,) ig-mcp-server$(if $(findstring windows,$*),.exe,)

ig-mcp-server-%: phony_explicit
	export GO111MODULE=on CGO_ENABLED=0 && \
	export GOOS=$(shell echo $* | cut -f1 -d-) GOARCH=$(shell echo $* | cut -f2 -d-) && \
	go build -ldflags $(LDFLAGS) \
		-tags withoutebpf \
		-o ig-mcp-server-$${GOOS}-$${GOARCH}$(if $(findstring windows,$*),.exe,) \
		github.com/inspektor-gadget/ig-mcp-server/cmd/ig-mcp-server

clean:
	@echo "Cleaning up ig-mcp-server binaries..."
	rm -f ig-mcp-server
	@echo "Clean completed."

clean-all: clean
	@echo "Cleaning up all ig-mcp-server binaries..."
	rm -f $(IG_MCP_SERVER_TARGET)
	@echo "Clean all completed."

.PHONY: lint
lint:
	echo "Running linter..."
	docker run --rm --env XDG_CACHE_HOME=/tmp/xdg_home_cache \
		--env GOLANGCI_LINT_CACHE=/tmp/golangci_lint_cache \
		--user $(shell id -u):$(shell id -g) -v $(shell pwd):/app -w /app \
		golangci/golangci-lint:$(LINTER_VERSION) golangci-lint run --fix
	@echo "Linting completed."

.PHONY: container
container:
	@echo "Building container image..."
	docker buildx build -t $(CONTAINER_REPO_NAMESPACE)/$(CONTAINER_REPO_NAME):$(IMAGE_TAG) --build-arg VERSION=$(VERSION) .
	@echo "Successfully built container image: $(CONTAINER_REPO_NAMESPACE)/$(CONTAINER_REPO_NAME):$(IMAGE_TAG)"

.PHONY: push-container
push-container: container
	@echo "Pushing container image to repository..."
	docker push $(CONTAINER_REPO_NAMESPACE)/$(CONTAINER_REPO_NAME):$(IMAGE_TAG)
	@echo "Successfully pushed container image: $(CONTAINER_REPO_NAMESPACE)/$(CONTAINER_REPO_NAME):$(IMAGE_TAG)"

.PHONY: clean-container
clean-container:
	@echo "Cleaning up..."
	docker rmi $(CONTAINER_REPO_NAMESPACE)/$(CONTAINER_REPO_NAME):$(IMAGE_TAG) || true
	@echo "Clean completed."

.PHONY: debug
debug:
	@echo "Running in inspector for debugging..."
	npx @modelcontextprotocol/inspector go run ./cmd/ig-mcp-server/ -gadget-images=$(GADGET_IMAGES)
