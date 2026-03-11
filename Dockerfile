FROM golang:1.26-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /out/syms .

FROM debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates libc6 \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/syms /usr/local/bin/syms

# Default to MCP stdio mode for server inspection and runtime use.
ENTRYPOINT ["/usr/local/bin/syms"]
CMD ["mcp"]
