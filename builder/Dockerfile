FROM rust:1.91-alpine AS builder

RUN apk add --no-cache musl-dev openssl-dev openssl-libs-static pkgconf git protobuf-dev

WORKDIR /app
COPY Cargo.toml Cargo.lock ./
RUN mkdir src && echo "fn main() {}" > src/main.rs && cargo build --release && rm -rf src

COPY build.rs ./
COPY proto/ proto/
COPY src/ src/
RUN touch src/main.rs && cargo build --release

FROM alpine:3.21

RUN apk add --no-cache git ca-certificates

COPY --from=builder /app/target/release/builder /usr/local/bin/builder

RUN adduser -D -u 1000 builder
USER builder

ENV RUST_LOG=info
ENTRYPOINT ["builder"]
