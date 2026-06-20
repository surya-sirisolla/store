# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Build both binaries: the API server and the MCP server.
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server && \
    CGO_ENABLED=0 go build -o /out/mcp ./cmd/mcp

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/server /usr/local/bin/server
COPY --from=build /out/mcp /usr/local/bin/mcp
EXPOSE 8080 9090
# Default command runs the API server; the MCP service overrides this with `mcp`.
CMD ["server"]
