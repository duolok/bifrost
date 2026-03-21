defmodule RealtimeWeb.ActivityChannel do
  use Phoenix.Channel

  @impl true
  def join("activity:feed", _params, socket) do
    Phoenix.PubSub.subscribe(Realtime.PubSub, "deploy:feed")
    {:ok, socket}
  end

  @impl true
  def handle_info({:event, event}, socket) do
    push(socket, "new_event", %{
      event_type: event.event_type,
      deploy_id: event.deploy_id,
      project_name: event.project_name,
      message: event.message,
      timestamp: event.timestamp
    })

    {:noreply, socket}
  end
end
