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

let () =
  let server = Tiny_httpd.create ~addr:"0.0.0.0" ~port () in
  Tiny_httpd.add_route_handler server ~meth:`GET
    Tiny_httpd.Route.(exact "health" @/ return)
    (fun _req -> Tiny_httpd.Response.make_string (Ok {|{"status": "ok"}|}));

  Tiny_httpd.add_route_handler server ~meth:`POST
    Tiny_httpd.Route.(exact "validate" @/ return)
    (fun req ->
      let body = Tiny_httpd.Request.body req in
      let result =
        match parse body with
        | Valid config -> validate config
        | Invalid errors -> Invalid errors
      in
      Tiny_httpd.Response.make_string (Ok (response_to_json result)));
  Printf.printf "Validator listening on port: %d\n%!" port;
  Tiny_httpd.run_exn server
