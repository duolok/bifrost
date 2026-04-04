defmodule Realtime.GRPC.EventIngress do
  use GRPC.Server, service: Bifrost.Events.EventIngress.Service
  require OpenTelemetry.Tracer, as: Tracer

  alias Bifrost.Events.{PlatformEvent, Ack}
  alias Realtime.EventDispatcher

  def send_event(%PlatformEvent{} = event, stream) do
    extract_trace_context(stream)

    Tracer.with_span "grpc.EventIngress/SendEvent" do
      Tracer.set_attributes([
        {"event.type", event.event_type},
        {"event.deploy_id", event.deploy_id}
      ])

      EventDispatcher.dispatch_event(event)
    end

    %Ack{}
  end

  def stream_build_logs(stream, grpc_stream) do
    extract_trace_context(grpc_stream)

    Tracer.with_span "grpc.EventIngress/StreamBuildLogs" do
      Enum.each(stream, fn log ->
        EventDispatcher.dispatch_log(log)
      end)
    end

    %Ack{}
  end

  defp extract_trace_context(stream) do
    case GRPC.Stream.get_headers(stream) do
      headers when is_map(headers) ->
        # Extract traceparent from gRPC metadata for trace propagation
        case Map.get(headers, "traceparent") do
          nil -> :ok
          traceparent ->
            :otel_propagator_text_map.extract([{"traceparent", traceparent}])
        end

      _ ->
        :ok
    end
  end
end
