<p align="center">
  <img src="https://img.shields.io/badge/OCaml-5.x-EC6813?style=for-the-badge&logo=ocaml&logoColor=white" alt="OCaml">
</p>

<h1 align="center">Validator</h1>

<p align="center">
  Deployment config parser and validator for Bifrost.
</p>

---

## Overview

The validator is an OCaml service that parses and validates `deploy.toml` configuration files before they enter the deployment pipeline. It guards the `queued → validating` transition in the Bifrost state machine — if the config is invalid, the deployment is rejected with structured errors before any resources are spent building or deploying.

The Go gateway sends the raw TOML config over HTTP. The validator parses it into typed records, checks every field against validation rules, and returns either a validated config or a list of errors with field names and messages.

## How It Works

1. The gateway fetches `deploy.toml` from the user's repository at the exact commit SHA.
2. It POSTs the raw TOML string to the validator's `/validate` endpoint.
3. The validator parses the TOML into an OCaml record type, checking:
   - Required fields are present (`app.name`, `app.runtime`)
   - Numeric fields are within valid ranges (replicas, CPU targets)
   - Resource formats are valid (`500m` CPU, `256Mi` memory)
   - Health check paths start with `/`
   - Interval/timeout strings parse correctly
4. If valid, returns the structured config as JSON. If invalid, returns an array of errors with the field name and message for each problem.

## Project Structure

```
validator/
├── bin/
│   └── main.ml           # HTTP server entrypoint (Dream framework)
├── lib/
│   ├── config.ml          # Typed config records and TOML parsing
│   ├── validate.ml        # Validation rules for each config section
│   ├── validator.mli      # Module interface
│   └── validator.ml       # Top-level validate function
├── test/
│   └── test_validator.ml  # Property-based and unit tests
├── dune-project           # Dune build system project file
├── k8s/
│   └── (manifests)
└── Dockerfile
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8090` | HTTP server port |

## Dependencies

| Library | Purpose |
|---------|---------|
| [dream](https://aantron.github.io/dream/) | HTTP framework — lightweight, modern OCaml web framework with good ergonomics. |
| [toml](https://github.com/ocaml-toml/To.ml) | TOML parsing for `deploy.toml` files. |
| [yojson](https://github.com/ocaml-community/yojson) | JSON serialization for API responses. |
| [alcotest](https://github.com/mirage/alcotest) | Testing framework for unit and property-based tests. |

## Design Decisions

**Why OCaml for validation?** OCaml's algebraic data types model the config structure as a precise AST — each config section becomes a variant or record type. Pattern matching ensures every field combination is handled exhaustively (the compiler flags unhandled cases). The type system makes it impossible to construct an invalid config object, so if parsing succeeds, the config is guaranteed valid.

**Why HTTP instead of gRPC?** The OCaml gRPC ecosystem is immature. The payload is small (a TOML string in, JSON out), the call is synchronous, and HTTP/JSON is simple to implement on both sides. No meaningful benefit from gRPC here.

## Running

```bash
# Build
cd validator && opam exec -- dune build

# Run
cd validator && opam exec -- dune exec bin/main.exe

# From repository root
make validator-build
make validator-run
```
