defmodule RealtimeWeb.UserSocket do
  use Phoenix.Socket

  channel "deploy:status:*", RealtimeWeb.DeployChannel
  channel "build:logs:*", RealtimeWeb.BuildChannel
  channel "activity:feed", RealtimeWeb.ActivityChannel

  @impl true
  def connect(_params, socket, _connect_info) do
    {:ok, socket}
  end

  @impl true
  def id(_socket) do
    nil
  end
end
