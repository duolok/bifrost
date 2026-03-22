defmodule RealtimeWeb.HealthController do
  use RealtimeWeb, :controller

  def index(conn, _params) do
    json(conn, %{status: "ok", service: "bifrost-realtime"})
  end
end
