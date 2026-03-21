defmodule RealtimeWeb.BuildChannel do
  use Phoenix.Channel

  @impl true
  def join("build:logs:" <> deploy_id, _params, socket) do
    Phoenix.PubSub.subscribe(Realtime.PubSub, "deploy:status:#{deploy_id}")
    {:ok, socket}
  end

  @impl true
  def handle_info({:log, log}, socket) do
    push(socket, "new_log", %{
      deploy_id: log.deploy_id,
      line: log.line,
      timestamp: log.timestamp
    })

    {:noreply, socket}
  end
end
