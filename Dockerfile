# -------------------------------------------------------------
# Build Stage
# -------------------------------------------------------------
FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Compile optimized statically linked binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /app/gateway \
    ./cmd/gateway

# -------------------------------------------------------------
# Production Runtime Stage (Minimal image < 20MB)
# -------------------------------------------------------------
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 10001 -S appgroup && \
    adduser -u 10001 -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/gateway /app/gateway

ENV PORT=8000 \
    ENVIRONMENT=production

EXPOSE 8000

USER 10001:10001

ENTRYPOINT ["/app/gateway"]
