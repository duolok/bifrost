FROM alpine:3.21 AS builder

RUN apk add --no-cache curl xz && \
    curl -sSLo /tmp/zig.tar.xz https://ziglang.org/download/0.14.0/zig-linux-x86_64-0.14.0.tar.xz && \
    tar -xJf /tmp/zig.tar.xz -C /usr/local && \
    ln -s /usr/local/zig-linux-x86_64-0.14.0/zig /usr/local/bin/zig && \
    rm /tmp/zig.tar.xz

WORKDIR /app
COPY build.zig ./
COPY src/ src/

RUN zig build -Doptimize=ReleaseSmall

FROM alpine:3.21

RUN adduser -D -u 1000 healthcheck
USER healthcheck

COPY --from=builder /app/zig-out/bin/healthcheck /usr/local/bin/healthcheck

ENTRYPOINT ["healthcheck"]
