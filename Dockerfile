FROM golang:1.24.2-bullseye@sha256:f0fe88a509ede4f792cbd42056e939c210a1b2be282cfe89c57a654ef8707cd2 AS builder

# Copy the source code
COPY . /mcp-server/
WORKDIR /mcp-server

# Build the inspektor-gadget-mcp-server binary
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o mcp-server ./cmd/inspektor-gadget-mcp-server

# Final image
FROM scratch
COPY --from=builder /mcp-server/mcp-server /mcp-server/server
ENTRYPOINT ["/mcp-server/server"]