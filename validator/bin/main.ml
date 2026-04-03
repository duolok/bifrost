open Bifrost_validator.Config
open Bifrost_validator.Parser
open Bifrost_validator.Validate

let port =
  match Sys.getenv_opt "BIFROST_VALIDATOR_PORT" with
  | Some value -> int_of_string value
  | None -> 8090

let error_to_json (e : validation_error) =
  `Assoc
    [
      ("field", `String e.field);
      ("message", `String e.message);
      ("line", `Int e.line);
    ]

let response_to_json (result : validation_result) =
  let json =
    match result with
    | Valid _ -> `Assoc [ ("status", `String "valid") ]
    | Invalid errors ->
        `Assoc
          [
            ("status", `String "invalid");
            ("errors", `List (List.map error_to_json errors));
          ]
  in
  Yojson.Safe.to_string json

let extract_trace_id (req : _ Tiny_httpd.Request.t) =
  let traceparent =
    match Tiny_httpd.Request.get_header req "traceparent" with
    | Some v -> v
    | None -> ""
  in
  (* W3C traceparent format: 00-<trace_id>-<span_id>-<flags> *)
  if String.length traceparent >= 55 then
    String.sub traceparent 3 32
  else
    ""

let log_request meth path trace_id status_code =
  let trace_field =
    if trace_id = "" then "" else Printf.sprintf {|,"trace_id":"%s"|} trace_id
  in
  Printf.printf {|{"method":"%s","path":"%s","status":%d%s}|} meth path status_code trace_field;
  print_newline ()

let () =
  let server = Tiny_httpd.create ~addr:"0.0.0.0" ~port () in
  Tiny_httpd.add_route_handler server ~meth:`GET
    Tiny_httpd.Route.(exact "health" @/ return)
    (fun _req -> Tiny_httpd.Response.make_string (Ok {|{"status": "ok"}|}));

  Tiny_httpd.add_route_handler server ~meth:`POST
    Tiny_httpd.Route.(exact "validate" @/ return)
    (fun req ->
      let trace_id = extract_trace_id req in
      let body = Tiny_httpd.Request.body req in
      let result =
        match parse body with
        | Valid config -> validate config
        | Invalid errors -> Invalid errors
      in
      let resp_body = response_to_json result in
      log_request "POST" "/validate" trace_id 200;
      let resp = Tiny_httpd.Response.make_string (Ok resp_body) in
      if trace_id <> "" then
        Tiny_httpd.Response.set_header "traceparent"
          (Printf.sprintf "00-%s-0000000000000000-01" trace_id) resp
      else
        resp);
  Printf.printf "Validator listening on port: %d\n%!" port;
  Tiny_httpd.run_exn server
