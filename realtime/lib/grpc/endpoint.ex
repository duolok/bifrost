defmodule Realtime.GRPC.Endpoint do
  use GRPC.Endpoint

  intercept GRPC.Server.Interceptors.Logger
  run(Realtime.GRPC.EventIngress)
end
