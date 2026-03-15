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
            | None -> {field = "name"; message = "name does not exist"; line = 1 } :: errors
            | Some _ -> errors
        in

        let errors = match runtime with 
            | None -> {field = "runtime"; message = "runtime does not exist"; line = 1 } :: errors
            | Some _ -> errors
        in

        match errors with 
        | [] -> Ok { name = Option.get name; runtime = Option.get runtime }
        | _ -> Error errors

let parse_scaling table = 
    match get_table "scaling" table with
        | None -> Error [{ field = "scaling"; message = "section is required"; line = 0 }]
        | Some t -> 
            let errors = [] in
            let min_replicas = get_int "min_replicas" t in
            let max_replicas = get_int "max_replicas" t in 
            let cpu_target = get_int "cpu_target" t in

            let errors = match min_replicas with 
                | None -> {field = "min_replicas"; message = "min_replicas does not exist"; line = 1} :: errors
                | Some _ ->  errors
            in

            let errors = match max_replicas with 
                | None -> {field = "max_replicas"; message = "max_replicas does not exist"; line = 1} :: errors
                | Some _ ->  errors
            in

            let errors = match cpu_target with
                | None -> {field = "cpu_target"; message = "cpu_target does not exist"; line = 1} :: errors
                | Some _ -> errors
            in

            match errors with
            | [] -> Ok { min_replicas = Option.get min_replicas; max_replicas = Option.get max_replicas; cpu_target = Option.get cpu_target }
            | _ -> Error errors

(* Replaces string duration with a \0 *)
let parse_duration s =
    let len = String.length s  in
    if len > 1 && s.[len-1] = 's' then
        int_of_string_opt(String.sub s 0 (len-1))
    else
        None

let parse_health table =
    match get_table "health" table with 
        | None -> Error [{ field = "health"; message = "health is required"; line = 0 }]
        | Some t -> 
            let errors = [] in
            let path = get_string "path" t in
            
            let get_duration key table =
                match get_string key table with 
                | Some s -> parse_duration s
                | None -> None
            in

            let interval = get_duration "interval" t in
            let timeout = get_duration "timeout" t in

            let errors = match path with 
                | None -> {field = "path"; message = "path does not exist"; line = 1} :: errors
                | Some _ -> errors
            in
            let errors = match interval with 
                | None -> {field = "interval"; message = "interval does not exist"; line = 1} :: errors
                | Some _ -> errors
            in
            let errors = match timeout with 
                | None -> {field = "timeout"; message = "timeout does not exist"; line = 1} :: errors
                | Some _ -> errors
            in

            match errors with
            | [] -> Ok { path = Option.get path; interval_seconds = Option.get interval; timeout_seconds = Option.get timeout }
            | _ -> Error errors
        

let parse (raw: string) : validation_result = 
    match Toml.Parser.from_string raw with
    | `Error (msg, _loc) -> 
      Invalid [{ field = "toml"; message = msg; line = 0 }]
    | `Ok table ->
      (* TODO: call parse_app, parse_scaling, etc. and combine results *)
;;
