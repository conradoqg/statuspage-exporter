###
# Multi-stage Dockerfile for building and running the statuspage-exporter
###
FROM golang:1.21-alpine AS builder

WORKDIR /src

# Cache deps
COPY go.mod ./
RUN go mod download

# Copy the full source
COPY . .

# Build static binary
RUN GOCACHE=/src/.gocache CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/statuspage-exporter ./cmd/statuspage-exporter

FROM alpine:3.19 AS runtime
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/statuspage-exporter /app/statuspage-exporter
COPY config.example.yaml /app/config.yaml

EXPOSE 8080
ENTRYPOINT ["/app/statuspage-exporter"]
CMD ["--config=/app/config.yaml", "--listen=:8080"]

