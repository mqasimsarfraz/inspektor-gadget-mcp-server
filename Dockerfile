FROM golang:1.24.2-bullseye@sha256:f0fe88a509ede4f792cbd42056e939c210a1b2be282cfe89c57a654ef8707cd2 AS builder

# Cache go modules so they won't be downloaded at each build
COPY go.mod go.sum /mcp-server/
RUN cd /mcp-server && go mod download

# Copy the source code
COPY . /mcp-server/
WORKDIR /mcp-server

# Build the inspektor-gadget-mcp-server binary
RUN CGO_ENABLED=0 GOOS=linux go build -o mcp-server ./cmd/inspektor-gadget-mcp-server

# Final image
FROM scratch
COPY --from=builder /mcp-server/mcp-server /mcp-server/
ENTRYPOINT ["/mcp-server/server"]