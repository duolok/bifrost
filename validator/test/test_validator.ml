open Bifrost_validator.Config
open Bifrost_validator.Parser
open Bifrost_validator.Validate

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

(* ===== Validation tests ===== *)

(* Helper: a valid config record to use as a base *)
let base_config : validated_config = {
    app = { name = "my-api"; runtime = "go" };
    scaling = { min_replicas = 2; max_replicas = 10; cpu_target = 70 };
    health = { path = "/healthz"; interval_seconds = 30; timeout_seconds = 5 };
    resources = { cpu = "500m"; memory = "256Mi" };
    env = [];
    routes = None;
}

let has_field_error field errors =
    List.exists (fun (e : validation_error) -> e.field = field) errors

(* -- validate_app_name tests -- *)

let test_valid_app_name () =
    Alcotest.(check int) "no errors" 0 (List.length (validate_app_name base_config))

let test_empty_app_name () =
    let cfg = { base_config with app = { name = ""; runtime = "go" } } in
    let errors = validate_app_name cfg in
    Alcotest.(check bool) "has error" true (has_field_error "app.name" errors)

let test_long_app_name () =
    let cfg = { base_config with app = { name = String.make 64 'a'; runtime = "go" } } in
    let errors = validate_app_name cfg in
    Alcotest.(check bool) "has error" true (has_field_error "app.name" errors)

let test_app_name_starts_with_hyphen () =
    let cfg = { base_config with app = { name = "-my-api"; runtime = "go" } } in
    let errors = validate_app_name cfg in
    Alcotest.(check bool) "has error" true (has_field_error "app.name" errors)

let test_app_name_ends_with_hyphen () =
    let cfg = { base_config with app = { name = "my-api-"; runtime = "go" } } in
    let errors = validate_app_name cfg in
    Alcotest.(check bool) "has error" true (has_field_error "app.name" errors)

let test_app_name_uppercase () =
    let cfg = { base_config with app = { name = "My-Api"; runtime = "go" } } in
    let errors = validate_app_name cfg in
    Alcotest.(check bool) "has error" true (has_field_error "app.name" errors)

let test_app_name_with_spaces () =
    let cfg = { base_config with app = { name = "my api"; runtime = "go" } } in
    let errors = validate_app_name cfg in
    Alcotest.(check bool) "has error" true (has_field_error "app.name" errors)

let test_app_name_max_length () =
    let cfg = { base_config with app = { name = String.make 63 'a'; runtime = "go" } } in
    Alcotest.(check int) "no errors" 0 (List.length (validate_app_name cfg))

(* -- validate_app_runtime tests -- *)

let test_valid_runtime () =
    Alcotest.(check int) "no errors" 0 (List.length (validate_app_runtime base_config))

let test_invalid_runtime () =
    let cfg = { base_config with app = { name = "my-api"; runtime = "docker" } } in
    let errors = validate_app_runtime cfg in
    Alcotest.(check bool) "has error" true (has_field_error "app.runtime" errors)

let test_all_valid_runtimes () =
    List.iter (fun rt ->
        let cfg = { base_config with app = { name = "my-api"; runtime = rt } } in
        Alcotest.(check int) ("no errors for " ^ rt) 0 (List.length (validate_app_runtime cfg))
    ) ["rust"; "go"; "elixir"; "python"; "zig"; "ocaml"; "node"]

(* -- validate_scaling tests -- *)

let test_valid_scaling () =
    Alcotest.(check int) "no errors" 0 (List.length (validate_scaling base_config))

let test_scaling_min_zero () =
    let cfg = { base_config with scaling = { min_replicas = 0; max_replicas = 10; cpu_target = 70 } } in
    let errors = validate_scaling cfg in
    Alcotest.(check bool) "has error" true (has_field_error "scaling.min_replicas" errors)

let test_scaling_max_less_than_min () =
    let cfg = { base_config with scaling = { min_replicas = 5; max_replicas = 3; cpu_target = 70 } } in
    let errors = validate_scaling cfg in
    Alcotest.(check bool) "has error" true (has_field_error "scaling.max_replicas" errors)

let test_scaling_cpu_target_zero () =
    let cfg = { base_config with scaling = { min_replicas = 1; max_replicas = 10; cpu_target = 0 } } in
    let errors = validate_scaling cfg in
    Alcotest.(check bool) "has error" true (has_field_error "scaling.cpu_target" errors)

let test_scaling_cpu_target_over_100 () =
    let cfg = { base_config with scaling = { min_replicas = 1; max_replicas = 10; cpu_target = 101 } } in
    let errors = validate_scaling cfg in
    Alcotest.(check bool) "has error" true (has_field_error "scaling.cpu_target" errors)

let test_scaling_cpu_target_boundaries () =
    let cfg1 = { base_config with scaling = { min_replicas = 1; max_replicas = 10; cpu_target = 1 } } in
    let cfg100 = { base_config with scaling = { min_replicas = 1; max_replicas = 10; cpu_target = 100 } } in
    Alcotest.(check int) "cpu_target=1 ok" 0 (List.length (validate_scaling cfg1));
    Alcotest.(check int) "cpu_target=100 ok" 0 (List.length (validate_scaling cfg100))

(* -- validate_health tests -- *)

let test_valid_health () =
    Alcotest.(check int) "no errors" 0 (List.length (validate_health base_config))

let test_health_path_no_slash () =
    let cfg = { base_config with health = { path = "healthz"; interval_seconds = 30; timeout_seconds = 5 } } in
    let errors = validate_health cfg in
    Alcotest.(check bool) "has error" true (has_field_error "health.path" errors)

let test_health_interval_zero () =
    let cfg = { base_config with health = { path = "/"; interval_seconds = 0; timeout_seconds = 5 } } in
    let errors = validate_health cfg in
    Alcotest.(check bool) "has error" true (has_field_error "health.interval" errors)

let test_health_timeout_zero () =
    let cfg = { base_config with health = { path = "/"; interval_seconds = 30; timeout_seconds = 0 } } in
    let errors = validate_health cfg in
    Alcotest.(check bool) "has error" true (has_field_error "health.timeout" errors)

let test_health_timeout_equals_interval () =
    let cfg = { base_config with health = { path = "/"; interval_seconds = 10; timeout_seconds = 10 } } in
    let errors = validate_health cfg in
    Alcotest.(check bool) "has error" true (has_field_error "health.timeout" errors)

let test_health_timeout_greater_than_interval () =
    let cfg = { base_config with health = { path = "/"; interval_seconds = 5; timeout_seconds = 10 } } in
    let errors = validate_health cfg in
    Alcotest.(check bool) "has error" true (has_field_error "health.timeout" errors)

(* -- validate_resources tests -- *)

let test_valid_resources () =
    Alcotest.(check int) "no errors" 0 (List.length (validate_resources base_config))

let test_resources_cpu_millicores () =
    let cfg = { base_config with resources = { cpu = "250m"; memory = "256Mi" } } in
    Alcotest.(check int) "no errors" 0 (List.length (validate_resources cfg))

let test_resources_cpu_whole () =
    let cfg = { base_config with resources = { cpu = "2"; memory = "256Mi" } } in
    Alcotest.(check int) "no errors" 0 (List.length (validate_resources cfg))

let test_resources_invalid_cpu () =
    let cfg = { base_config with resources = { cpu = "abc"; memory = "256Mi" } } in
    let errors = validate_resources cfg in
    Alcotest.(check bool) "has error" true (has_field_error "resources.cpu" errors)

let test_resources_zero_cpu () =
    let cfg = { base_config with resources = { cpu = "0m"; memory = "256Mi" } } in
    let errors = validate_resources cfg in
    Alcotest.(check bool) "has error" true (has_field_error "resources.cpu" errors)

let test_resources_memory_mi () =
    let cfg = { base_config with resources = { cpu = "500m"; memory = "512Mi" } } in
    Alcotest.(check int) "no errors" 0 (List.length (validate_resources cfg))

let test_resources_memory_gi () =
    let cfg = { base_config with resources = { cpu = "500m"; memory = "2Gi" } } in
    Alcotest.(check int) "no errors" 0 (List.length (validate_resources cfg))

let test_resources_invalid_memory () =
    let cfg = { base_config with resources = { cpu = "500m"; memory = "256MB" } } in
    let errors = validate_resources cfg in
    Alcotest.(check bool) "has error" true (has_field_error "resources.memory" errors)

let test_resources_zero_memory () =
    let cfg = { base_config with resources = { cpu = "500m"; memory = "0Mi" } } in
    let errors = validate_resources cfg in
    Alcotest.(check bool) "has error" true (has_field_error "resources.memory" errors)

(* -- validate_env tests -- *)

let test_valid_env_vars () =
    let cfg = { base_config with env = [("LOG_LEVEL", Plain "info"); ("DB_HOST", Plain "localhost")] } in
    Alcotest.(check int) "no errors" 0 (List.length (validate_env cfg))

let test_env_starts_with_digit () =
    let cfg = { base_config with env = [("1BAD_KEY", Plain "val")] } in
    let errors = validate_env cfg in
    Alcotest.(check bool) "has error" true (has_field_error "env.1BAD_KEY" errors)

let test_env_lowercase () =
    let cfg = { base_config with env = [("bad_key", Plain "val")] } in
    let errors = validate_env cfg in
    Alcotest.(check bool) "has error" true (has_field_error "env.bad_key" errors)

let test_env_with_hyphen () =
    let cfg = { base_config with env = [("BAD-KEY", Plain "val")] } in
    let errors = validate_env cfg in
    Alcotest.(check bool) "has error" true (has_field_error "env.BAD-KEY" errors)

let test_env_empty_is_ok () =
    Alcotest.(check int) "no errors" 0 (List.length (validate_env base_config))

(* -- validate (main function) tests -- *)

let test_validate_valid_config () =
    match validate base_config with
    | Valid _ -> ()
    | Invalid errors ->
        let msgs = List.map (fun e -> e.field ^ ": " ^ e.message) errors in
        Alcotest.fail (String.concat ", " msgs)

let test_validate_collects_multiple_errors () =
    let cfg = { base_config with
        app = { name = ""; runtime = "docker" };
        scaling = { min_replicas = 0; max_replicas = 0; cpu_target = 0 };
    } in
    match validate cfg with
    | Invalid errors ->
        Alcotest.(check bool) "multiple errors" true (List.length errors > 2)
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
        "validate app_name", [
            Alcotest.test_case "valid name" `Quick test_valid_app_name;
            Alcotest.test_case "empty name" `Quick test_empty_app_name;
            Alcotest.test_case "name too long" `Quick test_long_app_name;
            Alcotest.test_case "starts with hyphen" `Quick test_app_name_starts_with_hyphen;
            Alcotest.test_case "ends with hyphen" `Quick test_app_name_ends_with_hyphen;
            Alcotest.test_case "uppercase chars" `Quick test_app_name_uppercase;
            Alcotest.test_case "spaces in name" `Quick test_app_name_with_spaces;
            Alcotest.test_case "max length 63 ok" `Quick test_app_name_max_length;
        ];
        "validate app_runtime", [
            Alcotest.test_case "valid runtime" `Quick test_valid_runtime;
            Alcotest.test_case "invalid runtime" `Quick test_invalid_runtime;
            Alcotest.test_case "all valid runtimes" `Quick test_all_valid_runtimes;
        ];
        "validate scaling", [
            Alcotest.test_case "valid scaling" `Quick test_valid_scaling;
            Alcotest.test_case "min replicas zero" `Quick test_scaling_min_zero;
            Alcotest.test_case "max less than min" `Quick test_scaling_max_less_than_min;
            Alcotest.test_case "cpu target zero" `Quick test_scaling_cpu_target_zero;
            Alcotest.test_case "cpu target over 100" `Quick test_scaling_cpu_target_over_100;
            Alcotest.test_case "cpu target boundaries" `Quick test_scaling_cpu_target_boundaries;
        ];
        "validate health", [
            Alcotest.test_case "valid health" `Quick test_valid_health;
            Alcotest.test_case "path missing slash" `Quick test_health_path_no_slash;
            Alcotest.test_case "interval zero" `Quick test_health_interval_zero;
            Alcotest.test_case "timeout zero" `Quick test_health_timeout_zero;
            Alcotest.test_case "timeout equals interval" `Quick test_health_timeout_equals_interval;
            Alcotest.test_case "timeout greater than interval" `Quick test_health_timeout_greater_than_interval;
        ];
        "validate resources", [
            Alcotest.test_case "valid resources" `Quick test_valid_resources;
            Alcotest.test_case "cpu millicores" `Quick test_resources_cpu_millicores;
            Alcotest.test_case "cpu whole number" `Quick test_resources_cpu_whole;
            Alcotest.test_case "invalid cpu" `Quick test_resources_invalid_cpu;
            Alcotest.test_case "zero cpu" `Quick test_resources_zero_cpu;
            Alcotest.test_case "memory Mi" `Quick test_resources_memory_mi;
            Alcotest.test_case "memory Gi" `Quick test_resources_memory_gi;
            Alcotest.test_case "invalid memory" `Quick test_resources_invalid_memory;
            Alcotest.test_case "zero memory" `Quick test_resources_zero_memory;
        ];
        "validate env", [
            Alcotest.test_case "valid env vars" `Quick test_valid_env_vars;
            Alcotest.test_case "starts with digit" `Quick test_env_starts_with_digit;
            Alcotest.test_case "lowercase chars" `Quick test_env_lowercase;
            Alcotest.test_case "hyphen in key" `Quick test_env_with_hyphen;
            Alcotest.test_case "empty env ok" `Quick test_env_empty_is_ok;
        ];
        "validate main", [
            Alcotest.test_case "valid config passes" `Quick test_validate_valid_config;
            Alcotest.test_case "collects multiple errors" `Quick test_validate_collects_multiple_errors;
        ];
    ]
