open Config

let get_table key table =
    match Toml.Types.Table.find_opt (Toml.Min.key key) table with
        | Some (Toml.Types.TTable t) -> Some t
        | _ -> None
    ;;

let get_string key table =
    match Toml.Types.Table.find_opt (Toml.Min.key key) table with
        | Some (Toml.Types.TString s) -> Some s
        | _ -> None
    ;;

let get_int key table =
    match Toml.Types.Table.find_opt (Toml.Min.key key) table with 
        | Some(Toml.Types.TInt i) -> Some i
        | _ -> None
    ;;

let parse_app table =
    match get_table "app" table with
    | None -> Error [{ field = "app"; message = "section is required"; line = 0 }]
    | Some t -> 
        let errors = [] in
        let name = get_string "name" t in
        let runtime = get_string "runtime" t in
        let errors = match name with 
            | None -> {field = "name"; message = "name does not exist"; line=1 } :: errors
            | Some _ -> errors
        in
        let errors = match runtime with 
            | None -> {field = "runtime"; message = "runtime does not exist"; line=1 } :: errors
            | Some _ -> errors
        in
        match errors with 
        | [] -> Ok { name = Option.get name; runtime = Option.get runtime }
        | _ -> Error errors
            

let parse (raw: string) : validation_result = 
    match Toml.Parser.from_string raw with
    | `Error (msg, _loc) -> 
      Invalid [{ field = "toml"; message = msg; line = 0 }]
    | `Ok table ->
      (* TODO: call parse_app, parse_scaling, etc. and combine results *)
;;
