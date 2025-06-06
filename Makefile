GO ?= go

CONTAINER_REPO_NAMESPACE ?= ghcr.io/inspektor-gadget
CONTAINER_REPO_NAME ?= inspektor-gadget-mcp-server
IMAGE_TAG ?= latest

.DEFAULT_GOAL := build
.PHONY: build
build:
	@echo "Building container image..."
	docker build -t $(CONTAINER_REPO_NAMESPACE)/$(CONTAINER_REPO_NAME):$(IMAGE_TAG) .
	@echo "Successfully built container image: $(CONTAINER_REPO_NAMESPACE)/$(CONTAINER_REPO_NAME):$(IMAGE_TAG)"

.PHONY: push
push: build
	@echo "Pushing container image to repository..."
	docker push $(CONTAINER_REPO_NAMESPACE)/$(CONTAINER_REPO_NAME):$(IMAGE_TAG)
	@echo "Successfully pushed container image: $(CONTAINER_REPO_NAMESPACE)/$(CONTAINER_REPO_NAME):$(IMAGE_TAG)"

.PHONY: clean
clean:
	@echo "Cleaning up..."
	docker rmi $(CONTAINER_REPO_NAMESPACE)/$(CONTAINER_REPO_NAME):$(IMAGE_TAG) || true
	@echo "Clean completed."

build-local: clean-local
	@echo "Building the project..."
	mkdir -p bin
	$(GO) build -o bin/inspektor-gadget-mcp-server ./cmd/inspektor-gadget-mcp-server
	@echo "Build completed."

.PHONY: clean-local
clean-local:
	@echo "Cleaning up..."
	rm -rf bin
	@echo "Clean completed."
