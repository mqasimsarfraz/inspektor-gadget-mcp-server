GO ?= go

.DEFAULT_GOAL := build
.PHONY: build
build: clean
	@echo "Building the project..."
	mkdir -p bin
	$(GO) build -o bin/inspektor-gadget-mcp-server ./cmd/inspektor-gadget-mcp-server
	@echo "Build completed."

.PHONY: clean
clean:
	@echo "Cleaning up..."
	rm -rf bin
	@echo "Clean completed."

.PHONY: image
image:
	@echo "Building Docker image..."
	docker build -t inspektor-gadget-mcp-server:latest .
	@echo "Docker image built successfully."