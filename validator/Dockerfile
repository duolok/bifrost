FROM ocaml/opam:debian-ocaml-5.2 AS builder

WORKDIR /home/opam/app

RUN opam install dune toml yojson tiny_httpd -y

COPY --chown=opam:opam dune-project .
COPY --chown=opam:opam lib/ lib/
COPY --chown=opam:opam bin/ bin/

RUN eval $(opam env) && dune build bin/main.exe

FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends wget adduser && rm -rf /var/lib/apt/lists/* && \
    adduser --disabled-password --gecos "" --uid 1000 bifrost
USER bifrost
COPY --from=builder /home/opam/app/_build/default/bin/main.exe /usr/local/bin/validator

ENV BIFROST_VALIDATOR_PORT=8090
EXPOSE 8090
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD wget -q --spider http://localhost:8090/health || exit 1

CMD ["validator"]
