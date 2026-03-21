defmodule Realtime.GRPC.EventIngress do
  use GRPC.Server, service: Bifrost.Events.EventIngress.Service

  alias Bifrost.Events.{PlatformEvent, BuildLogLine, Ack}
  alias Realtime.EventDispatcher

  def send_event(%PlatformEvent{} = event, _stream) do
    EventDispatcher.dispatch_event(event)
    %Ack{}
  end

  def stream_build_logs(stream, _stream) do
    Enum.each(stream, fn log ->
      EventDispatcher.dispatch_log(log)
    end)

    %Ack{}
  end
end
