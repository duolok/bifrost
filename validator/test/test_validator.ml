open Bifrost_validator.Config
open Bifrost_validator.Parser

(* -- Test helpers -- *)

let valid_toml = {|
[app]
name = "my-api"
runtime = "docker"

[scaling]
min_replicas = 2
max_replicas = 10
cpu_target = 70

[health]
path = "/healthz"
interval = "30s"
timeout = "5s"

[resources]
cpu = "500m"
memory = "256Mi"

[env]
LOG_LEVEL = "info"

[routes]
lua_script = "routes.lua"
|}

(* -- Valid config tests -- *)

let test_valid_config () =
    match parse valid_toml with
    | Valid config ->
        Alcotest.(check string) "app name" "my-api" config.app.name;
        Alcotest.(check string) "app runtime" "docker" config.app.runtime;
        Alcotest.(check int) "min replicas" 2 config.scaling.min_replicas;
        Alcotest.(check int) "max replicas" 10 config.scaling.max_replicas;
        Alcotest.(check int) "cpu target" 70 config.scaling.cpu_target;
        Alcotest.(check string) "health path" "/healthz" config.health.path;
        Alcotest.(check int) "interval" 30 config.health.interval_seconds;
        Alcotest.(check int) "timeout" 5 config.health.timeout_seconds;
        Alcotest.(check string) "cpu" "500m" config.resources.cpu;
        Alcotest.(check string) "memory" "256Mi" config.resources.memory
    | Invalid errors ->
        let msgs = List.map (fun e -> e.field ^ ": " ^ e.message) errors in
        Alcotest.fail (String.concat ", " msgs)

let test_valid_routes () =
    match parse valid_toml with
    | Valid config ->
        (match config.routes with
         | Some r -> Alcotest.(check string) "lua script" "routes.lua" r.lua_script
         | None -> Alcotest.fail "expected routes to be Some")
    | Invalid _ -> Alcotest.fail "expected valid config"

let test_valid_env () =
    match parse valid_toml with
    | Valid config ->
        let has_log_level = List.exists (fun (k, v) ->
            k = "LOG_LEVEL" && v = Plain "info"
        ) config.env in
        Alcotest.(check bool) "has LOG_LEVEL" true has_log_level
    | Invalid _ -> Alcotest.fail "expected valid config"

(* -- Missing section tests -- *)

let test_missing_app () =
    let toml = {|
[scaling]
min_replicas = 1
max_replicas = 2
cpu_target = 50
[health]
path = "/"
interval = "10s"
timeout = "3s"
[resources]
cpu = "100m"
memory = "128Mi"
|} in
    match parse toml with
    | Invalid errors ->
        let has_app_error = List.exists (fun e -> e.field = "app") errors in
        Alcotest.(check bool) "has app error" true has_app_error
    | Valid _ -> Alcotest.fail "expected invalid"

let test_missing_scaling () =
    let toml = {|
[app]
name = "test"
runtime = "docker"
[health]
path = "/"
interval = "10s"
timeout = "3s"
[resources]
cpu = "100m"
memory = "128Mi"
|} in
    match parse toml with
    | Invalid errors ->
        let has_scaling_error = List.exists (fun e -> e.field = "scaling") errors in
        Alcotest.(check bool) "has scaling error" true has_scaling_error
    | Valid _ -> Alcotest.fail "expected invalid"

(* -- Optional section tests -- *)

let test_optional_routes () =
    let toml = {|
[app]
name = "test"
runtime = "docker"
[scaling]
min_replicas = 1
max_replicas = 2
cpu_target = 50
[health]
path = "/"
interval = "10s"
timeout = "3s"
[resources]
cpu = "100m"
memory = "128Mi"
|} in
    match parse toml with
    | Valid config ->
        Alcotest.(check bool) "routes is None" true (config.routes = None)
    | Invalid _ -> Alcotest.fail "expected valid config"

let test_optional_env () =
    let toml = {|
[app]
name = "test"
runtime = "docker"
[scaling]
min_replicas = 1
max_replicas = 2
cpu_target = 50
[health]
path = "/"
interval = "10s"
timeout = "3s"
[resources]
cpu = "100m"
memory = "128Mi"
|} in
    match parse toml with
    | Valid config ->
        Alcotest.(check int) "env is empty" 0 (List.length config.env)
    | Invalid _ -> Alcotest.fail "expected valid config"

(* -- Secret env test -- *)

let test_secret_env () =
    let toml = {|
[app]
name = "test"
runtime = "docker"
[scaling]
min_replicas = 1
max_replicas = 2
cpu_target = 50
[health]
path = "/"
interval = "10s"
timeout = "3s"
[resources]
cpu = "100m"
memory = "128Mi"
[env]
DB_URL = { secret = "db-conn" }
|} in
    match parse toml with
    | Valid config ->
        let has_secret = List.exists (fun (k, v) ->
            k = "DB_URL" && v = Secret "db-conn"
        ) config.env in
        Alcotest.(check bool) "has secret env" true has_secret
    | Invalid _ -> Alcotest.fail "expected valid config"

(* -- Invalid TOML test -- *)

let test_invalid_toml () =
    match parse "this is not valid toml =-=" with
    | Invalid errors ->
        let has_toml_error = List.exists (fun e -> e.field = "toml") errors in
        Alcotest.(check bool) "has toml parse error" true has_toml_error
    | Valid _ -> Alcotest.fail "expected invalid"

(* -- Missing fields test -- *)

let test_missing_fields () =
    let toml = {|
[app]
name = "test"
[scaling]
min_replicas = 1
[health]
path = "/"
[resources]
cpu = "100m"
|} in
    match parse toml with
    | Invalid errors ->
        Alcotest.(check bool) "has multiple errors" true (List.length errors > 1)
    | Valid _ -> Alcotest.fail "expected invalid"


let () =
    Alcotest.run "Bifrost Validator" [
        "valid config", [
            Alcotest.test_case "parses full valid config" `Quick test_valid_config;
            Alcotest.test_case "parses routes" `Quick test_valid_routes;
            Alcotest.test_case "parses env" `Quick test_valid_env;
        ];
        "missing sections", [
            Alcotest.test_case "missing app section" `Quick test_missing_app;
            Alcotest.test_case "missing scaling section" `Quick test_missing_scaling;
        ];
        "optional sections", [
            Alcotest.test_case "routes is optional" `Quick test_optional_routes;
            Alcotest.test_case "env is optional" `Quick test_optional_env;
        ];
        "env parsing", [
            Alcotest.test_case "parses secret env vars" `Quick test_secret_env;
        ];
        "error handling", [
            Alcotest.test_case "rejects invalid toml" `Quick test_invalid_toml;
            Alcotest.test_case "reports multiple missing fields" `Quick test_missing_fields;
        ];
    ]
