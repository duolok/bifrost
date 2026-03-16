open Config

let is_dns_char c = (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c = '-'
;;

let is_env_char c = (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c = '_'
;;

let available_runtimes = ["rust"; "go"; "elixir"; "python"; "zig"; "ocaml"; "node"]
let k8s_memory_formats = ["Mi"; "Gi"]

let validate_app_name (cfg : validated_config) =
  let name = cfg.app.name in
  let err msg = [{ field = "app.name"; message = msg; line = 0 }] in

  if String.length name = 0 then err "name cannot be empty"
  else if String.length name > 63 then err "name exceeds 63 characters"
  else if name.[0] = '-' || name.[String.length name - 1] = '-' then
      err "name cannot start or end with a hyphen"
  else if not (String.for_all is_dns_char name) then
      err "name must contain only lowercase letters, digits or hyphens"
  else []
;;

let validate_app_runtime (cfg: validated_config) =
  match List.mem cfg.app.runtime available_runtimes with 
  | true -> []
  | false -> [{ field = "app.runtime"; message = "runtime is not available"; line = 0 }]
;;

let validate_scaling (cfg: validated_config) =
  let s = cfg.scaling in
  let checks = [
    (s.min_replicas < 1, "scaling.min_replicas", "must be at least 1");
    (s.max_replicas < s.min_replicas, "scaling.max_replicas", "must be >= than min.replicas");
    (s.cpu_target < 1 || s.cpu_target > 100, "scaling.cpu_target", "must be between 1 and 100");
  ] in
  List.filter_map ( fun (failed, field, message) -> 
    if failed then Some { field; message; line = 0 }
    else None
  ) checks
;;

let validate_health (cfg: validated_config) =
  let h = cfg.health in
  let checks = [
    (not (String.starts_with ~prefix:"/" h.path), "health.path", "must start with /");
    (h.interval_seconds < 1, "health.interval", "must be greater than 0");
    (h.timeout_seconds < 1, "health.timeout", "must be greater than 0");
    (h.timeout_seconds >= h.interval_seconds, "health.timeout", "must be less than interval");
  ] in
  List.filter_map (fun (failed, field, message) ->
    if failed then Some { field; message; line = 0 }
    else None
  ) checks
;;

let validate_resources (cfg: validated_config) =
  let r = cfg.resources in
  let is_valid_cpu c =
    if String.ends_with ~suffix:"m" c then 
      let num = String.sub c 0 (String.length c - 1) in
      match int_of_string_opt num with 
      | Some n -> n > 0
      | None -> false
    else 
      match int_of_string_opt c with
      |Some n -> n > 0
      | None -> false
  in

  let is_valid_memory m =
    List.exists (fun suffix ->
    if String.ends_with ~suffix m then
      let num = String.sub m 0 (String.length m  - String.length suffix) in
      match int_of_string_opt num with
      | Some n -> n > 0
      | None -> false
    else
      false
  ) k8s_memory_formats
  in

  let checks = [
    (not(is_valid_memory r.memory), "resources.memory", "memory is not valid");
    (not(is_valid_cpu r.cpu), "resources.cpu", "cpu is not valid");
  ] in
  List.filter_map ( fun (failed, field, message) ->
    if failed then Some { field; message; line = 0 }
    else None
  ) checks
;;

let validate_env (cfg: validated_config) =
  List.concat_map (fun (key, _) ->
    let field = "env." ^ key in
    let checks = [
      (String.length key = 0, "must not be empty");
      (key.[0] >= '0' && key.[0] <= '9', "must not start with a digit");
      (not (String.for_all is_env_char key), "must contain only uppercase letters, digits or underscores");
    ] in
    List.filter_map (fun(failed, message) ->
      if failed then Some { field; message; line =0 }
      else None
    ) checks
  ) cfg.env
;;

let validate (cfg: validated_config) =
  let errors =
   validate_app_name cfg
      @ validate_app_runtime cfg
      @ validate_scaling cfg
      @ validate_health cfg
      @ validate_resources cfg
      @ validate_env cfg
    in match errors with
    | [] -> Valid cfg
    | _ -> Invalid errors
;;
