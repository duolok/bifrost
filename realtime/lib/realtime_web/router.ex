defmodule RealtimeWeb.Router do
  use RealtimeWeb, :router

  pipeline :api do
    plug :accepts, ["json"]
  end

  scope "/api", RealtimeWeb do
    pipe_through :api
  end
end
