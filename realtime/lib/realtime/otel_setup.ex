defmodule Realtime.OtelSetup do
  @moduledoc """
  Configures OpenTelemetry tracing for the realtime service.
  """

  require Logger

  def setup do
    endpoint = System.get_env("OTEL_EXPORTER_OTLP_ENDPOINT")

    if endpoint do
      Logger.info("OpenTelemetry tracing enabled, endpoint=#{endpoint}")
    else
      Logger.info("OTEL_EXPORTER_OTLP_ENDPOINT not set, tracing disabled")
    end
  end
end
