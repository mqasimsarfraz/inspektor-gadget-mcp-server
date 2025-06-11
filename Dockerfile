FROM golang:1.24.2-bullseye@sha256:f0fe88a509ede4f792cbd42056e939c210a1b2be282cfe89c57a654ef8707cd2 AS builder

# Copy the source code
COPY . /mcp-server/
WORKDIR /mcp-server

# Build the ig-mcp-server binary
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o mcp-server ./cmd/ig-mcp-server

FROM alpine:3.22.0@sha256:8a1f59ffb675680d47db6337b49d22281a139e9d709335b492be023728e11715 AS certificates
RUN apk add --no-cache ca-certificates

# Final image
FROM scratch

COPY --from=certificates /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /mcp-server/mcp-server /mcp-server/server

ENTRYPOINT ["/mcp-server/server"]