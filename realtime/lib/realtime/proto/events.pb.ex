defmodule Bifrost.Events.PlatformEvent.MetadataEntry do
  @moduledoc false

  use Protobuf,
    full_name: "bifrost.events.PlatformEvent.MetadataEntry",
    map: true,
    protoc_gen_elixir_version: "0.16.0",
    syntax: :proto3

  field :key, 1, type: :string
  field :value, 2, type: :string
end

defmodule Bifrost.Events.PlatformEvent do
  @moduledoc false

  use Protobuf,
    full_name: "bifrost.events.PlatformEvent",
    protoc_gen_elixir_version: "0.16.0",
    syntax: :proto3

  field :event_type, 1, type: :string, json_name: "eventType"
  field :deploy_id, 2, type: :string, json_name: "deployId"
  field :project_name, 3, type: :string, json_name: "projectName"
  field :actor, 4, type: :string
  field :message, 5, type: :string
  field :timestamp, 6, type: :int64
  field :metadata, 7, repeated: true, type: Bifrost.Events.PlatformEvent.MetadataEntry, map: true
end

defmodule Bifrost.Events.BuildLogLine do
  @moduledoc false

  use Protobuf,
    full_name: "bifrost.events.BuildLogLine",
    protoc_gen_elixir_version: "0.16.0",
    syntax: :proto3

  field :deploy_id, 1, type: :string, json_name: "deployId"
  field :line, 2, type: :string
  field :timestamp, 3, type: :int64
end

defmodule Bifrost.Events.Ack do
  @moduledoc false

  use Protobuf,
    full_name: "bifrost.events.Ack",
    protoc_gen_elixir_version: "0.16.0",
    syntax: :proto3
end

defmodule Bifrost.Events.EventIngress.Service do
  @moduledoc false

  use GRPC.Service, name: "bifrost.events.EventIngress", protoc_gen_elixir_version: "0.16.0"

  rpc :SendEvent, Bifrost.Events.PlatformEvent, Bifrost.Events.Ack

  rpc :StreamBuildLogs, stream(Bifrost.Events.BuildLogLine), Bifrost.Events.Ack
end

defmodule Bifrost.Events.EventIngress.Stub do
  @moduledoc false

  use GRPC.Stub, service: Bifrost.Events.EventIngress.Service
end
