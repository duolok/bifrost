defmodule Realtime.EventDispatcher do
  @pubsub Realtime.PubSub

  def dispatch_event(%Bifrost.Events.PlatformEvent{} = event) do
    Phoenix.PubSub.broadcast(@pubsub, "deploy:status:#{event.deploy_id}", {:event, event})
    Phoenix.PubSub.broadcast(@pubsub, "activity:feed", {:event, event})
  end

  def dispatch_log(%Bifrost.Events.BuildLogLine{} = log) do
    Phoenix.PubSub.broadcast(@pubsub, "build:logs:#{log.deploy_id}", {:log, log})
  end
end
