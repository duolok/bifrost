FROM ocaml/opam:debian-ocaml-5.2 AS builder

WORKDIR /home/opam/app

RUN opam install dune toml yojson tiny_httpd -y

COPY --chown=opam:opam dune-project .
COPY --chown=opam:opam lib/ lib/
COPY --chown=opam:opam bin/ bin/

RUN eval $(opam env) && dune build bin/main.exe

FROM debian:bookworm-slim

COPY --from=builder /home/opam/app/_build/default/bin/main.exe /usr/local/bin/validator

ENV BIFROST_VALIDATOR_PORT=8090
EXPOSE 8090

CMD ["validator"]
