# hexagonal-arch

A Claude Code plugin that teaches Claude to build Go services with hexagonal
architecture (ports and adapters) and scaffolds the structure for it.

The core (domain and use cases) declares ports in its own language. Adapters
in `infra/` plug HTTP, gRPC, databases, brokers and remote APIs into them, and
only `cmd/<service>/main.go` knows which adapters are used.

It pairs with [cqrs-eda](https://github.com/theskyinflames/golang-cqrs-eda):

| | cqrs-eda | hexagonal |
|---|---|---|
| Owns | The core: commands, queries, events, aggregates, policies, middleware | The edges: layout, ports, adapters, wiring, boundary tests |
| Without the other | Uses its own `domain/app/infra` layout | Use cases are plain `Handle` types in `app/` |
| Together | Commands and queries are the driving ports | Adapters call `cqrs.Send`/`cqrs.Ask`; no extra service layer |

Both use the same layout, so they don't fight over where code goes.

## Install

In Claude Code:

```
/plugin marketplace add theskyinflames/golang-cqrs-eda
/plugin install hexagonal@theskyinflames
```

Requires Go 1.24+. Examples use the Go 1.27 stdlib `uuid`; on older versions
Claude uses `github.com/google/uuid`.

## What you get

- **`hexagonal` skill.** Loads when you scaffold a Go service or bounded
  context, add or replace an adapter (HTTP, gRPC, CLI, consumer, database,
  remote API), wire `main.go`, or fix code where SQL or HTTP leaks into the
  core. Claude then follows the conventions: ports in the core's language,
  adapters that only translate, domain errors mapped at each boundary, one
  composition root, and tests per ring, including contract suites that every
  storage adapter must pass.
- **`scaffold.sh`.** Creates `internal/<context>/{domain,app,infra}` or a
  `cmd/<service>/main.go` composition root. Never overwrites, runs `go vet`.

Just describe the task, for example:

> start a new Go service for invoicing with a postgres store and an HTTP API

> add a Kafka consumer that triggers the lend-book use case

> our use cases call database/sql directly; put the storage behind a port

## Repository layout

| Path | Contents |
|---|---|
| `skills/hexagonal/` | The skill: `SKILL.md`, scaffold templates, `scaffold.sh`, `references/patterns.md` |
| `dev/` | `sample-library`, the compiled example that `gen-patterns.sh` turns into `patterns.md` |
| `STATUS.md` | Progress |

## License

MIT
