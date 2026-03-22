FROM elixir:1.18-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

ENV MIX_ENV=prod

RUN mix local.hex --force && mix local.rebar --force

COPY mix.exs mix.lock ./
RUN mix deps.get --only prod && mix deps.compile

COPY config/ config/
COPY lib/ lib/

RUN mix compile && mix release

FROM alpine:3.21

RUN apk add --no-cache libstdc++ openssl ncurses-libs && \
    adduser -D -u 1000 realtime

USER realtime
WORKDIR /app

COPY --from=builder --chown=realtime /app/_build/prod/rel/realtime ./

ENV PHX_SERVER=true
EXPOSE 4000 50051

CMD ["bin/realtime", "start"]
