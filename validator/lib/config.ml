type app_config = {
    name: string;
    runtime: string;
}

type env_value =
    | Plain of string
    | Secret of string

type scaling_config = {
    min_replicas: int;
    max_replicas: int;
    cpu_target: int;
}

type health_config = {
    path: string;
    interval_seconds: int;
    timeout_seconds: int;
}

type route_config = {
    lua_script: string;
}

type resources_config = {
    cpu: string;
    memory: string;
}

type validated_config = {
    app: app_config;
    scaling: scaling_config;
    health: health_config;
    resources: resources_config;
    env: (string * env_value) list;
    routes: route_config option;
}

type validation_error = {
    field: string;
    message: string;
    line: int;
}

type validation_result =
    | Valid of validated_config 
    | Invalid of validation_error list
