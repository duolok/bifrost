defmodule RealtimeWeb.DeployChannel do
  use Phoenix.Channel

  @impl true
  def join("deploy:status:" <> deploy_id, _params, socket) do
    Phoenix.PubSub.subscribe(Realtime.PubSub, "deploy:status:#{deploy_id}")
    {:ok, socket}
  end

  @impl true
  def handle_info({:event, event}, socket) do
    push(socket, "status_changed", %{
      event_type: event.event_type,
      deploy_id: event.deploy_id,
      project_name: event.project_name,
      message: event.message,
      timestamp: event.timestamp
    })

    {:noreply, socket}
  end
end
