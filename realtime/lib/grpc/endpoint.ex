defmodule Realtime.GRPC.Endpoint do
  use GRPC.Endpoint

  intercept GRPC.Server.Interceptor.Logger
  run(Realtime.GRPC.EventIngress)
end
