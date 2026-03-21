defmodule Realtime.GRPC.Server do
  def child_spec(_opts) do
    port = String.to_integer(System.get_env("GRPC_PORT") || "50051")

    GRPC.Server.Supervisor.child_spec(Realtime.GRPC.Endpoint, port)
  end
end
