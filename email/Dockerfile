FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o email-worker ./cmd/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/email-worker /usr/local/bin/email-worker
USER nobody
ENTRYPOINT ["email-worker"]
