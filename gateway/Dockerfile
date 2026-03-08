FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o gateway cmd/main.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 1000 bifrost
USER bifrost
COPY --from=builder /app/gateway /usr/local/bin/gateway
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

CMD ["gateway"]
