---
name: hexagonal
description: "Use this skill for the structure and boundaries of Go services built with hexagonal architecture (ports and adapters), even when none of those words appear. Trigger on: scaffolding a new Go service or bounded context; adding, replacing or testing an adapter — HTTP or gRPC handlers, CLI commands, message consumers, scheduled jobs, database repositories, in-memory stores, clients for external APIs or other services; putting an external dependency behind an interface so the core can be tested without it; wiring cmd/<service>/main.go; deciding which package new code belongs in; or fixing boundary problems such as domain or use-case code importing database/sql, net/http, a driver or an adapter package, or SQL/HTTP errors leaking into the core. Works alongside the cqrs-eda skill: when the project uses internal/platform/cqrs, cqrs-eda owns commands, queries, events and aggregates, and this skill owns the adapters and wiring around them. Skip non-Go code, single-file scripts, libraries with no I/O, and conceptual-only questions."
---

# Hexagonal architecture for Go

The core (domain + use cases) knows nothing about HTTP, SQL, brokers or remote
APIs. It declares **ports** — interfaces in its own language — and **adapters**
in `infra/` plug technologies into them. Only `cmd/<service>/main.go` knows
which adapters are used. This skill owns that structure: layout, ports,
adapters, wiring and the tests at each boundary.

## 1. Detect the project's mode first

```bash
grep -rl --include=*.go 'internal/platform/cqrs"' . | head -1
```

- **Found → CQRS mode.** Use cases are commands and queries dispatched through
  the bus. Load and follow the `cqrs-eda` skill for everything inside the core
  (commands, queries, events, aggregates, policies, middleware, unit of work).
  From this skill, apply the adapter, wiring and testing rules. Never add
  use-case structs or "service" layers beside the command/query handlers, and
  driving adapters call `cqrs.Send`/`cqrs.Ask`.
- **Not found → standalone mode.** Use cases are plain types in `app/` (rule 3).
  Don't install the CQRS platform unless the user asks for CQRS, events or the
  cqrs-eda conventions; if they do, hand over to the `cqrs-eda` skill.

If the project has its own layout, fit into it rather than imposing this one,
and don't move existing code unless the user asks.

## 2. Layout

Same tree as cqrs-eda, so both skills agree on where things go:

```
cmd/<service>/main.go          composition root: config, adapters, wiring
internal/
  <context>/                   one bounded context, e.g. lending, billing
    domain/                    entities, value objects, rules, domain errors, repository ports
    app/                       use cases (driving ports), read ports, ports to other systems
    infra/                     adapters, one package per technology:
      httpapi/ grpcapi/ cli/ consumer/        driving: call use cases
      postgres/ memory/ <remote>api/          driven: implement ports
      <port>test/                             contract suite for a port's adapters
  platform/                    only shared technical code (e.g. the cqrs-eda platform)
```

Name adapter packages after the technology plus a suffix when the bare name
would shadow a stdlib package (`httpapi`, not `http`).

Dependencies point inward: `cmd → infra → app → domain`. `domain` imports only
the standard library (plus `platform/{ddd,events}` in CQRS mode). `app` never
imports `infra`. A context never imports another context's `infra` or `app`
internals.

To scaffold, run (it never overwrites, then runs `go vet`):

```bash
bash <skill-dir>/scripts/scaffold.sh context <project-root> <name>   # internal/<name>/{domain,app,infra}
bash <skill-dir>/scripts/scaffold.sh service <project-root> <name>   # cmd/<name>/main.go, standalone mode only
```

In CQRS mode, write `main.go` from section 7 of the cqrs-eda patterns instead
of `scaffold.sh service`.

## 3. Rules, and why

1. **Ports belong to the core and speak its language.** Declare a driven port
   where it is used: repository ports in `domain`; read-model ports and ports
   to other systems in `app`. Name them for what the core needs
   (`MemberDirectory.IsActive`), not for the technology behind them
   (`MembersAPIClient.GetMember`). Signatures never contain `*sql.Tx`,
   `*http.Request`, protobuf messages, driver types or driver errors; that is
   what lets you swap or fake the adapter.
2. **Keep ports small.** A port has the methods its users call, nothing more.
   One adapter may implement several ports (a store implementing both the
   repository and the read port is normal).
3. **Use cases are the driving ports.** Standalone mode: one type per use case
   in `app/`, with `Handle(ctx, Input) error` for writes and
   `Handle(ctx, Input) (View, error)` for reads; inputs and views are plain
   data. No "service" structs grouping unrelated operations. Writes don't
   return data: the caller generates IDs up front and reads afterwards. This
   shape matches cqrs-eda's handlers, so moving to CQRS later is mechanical.
   CQRS mode: commands and queries are the driving ports.
4. **Driving adapters translate, nothing else.** Decode the input, check its
   format (not business rules), call one use case, map errors, encode the
   output. They own their transport shapes (JSON/proto DTOs), separate from
   the core's views. They depend on small interfaces they declare for the use
   cases they call, so they can be tested with fakes.
5. **Driven adapters translate, nothing else.** Map records to and from
   entities; store records, never the entity itself (it would share pointers,
   and in CQRS mode keep pending events). Translate driver and remote errors
   into domain errors (`sql.ErrNoRows` → `domain.ErrNotFound`, remote 404 →
   a domain error). Put timeouts on everything remote. Add a compile-time
   check: `var _ domain.BookRepository = (*Books)(nil)`.
6. **Errors cross the boundary as domain errors.** They are defined in
   `domain`; driven adapters return them; use cases wrap with `%w`; driving
   adapters map them with `errors.Is` in one function per adapter: not found →
   404/NotFound, invalid input → 400/InvalidArgument, rule violation →
   409/422/FailedPrecondition, anything else → 500/Internal. Unexpected errors
   are logged once, at the driving adapter (or by cqrs-eda's logging
   middleware in CQRS mode), never also inside the core.
7. **The composition root is the only place that knows concrete adapters.**
   `cmd/<service>/main.go` has a `run() error` that reads config (env, flags),
   opens connections, builds driven adapters, injects them into use cases and
   hands those to driving adapters, with graceful shutdown. No package-level
   state, no `init()` wiring, no service locator; a DI framework is
   unnecessary. Several driving adapters (HTTP and a consumer, say) share the
   same use-case values.
8. **Atomic writes go through a port.** When a use case must change several
   records atomically, declare a unit-of-work/transactor port in `app` and
   implement it in `infra/<db>`, carrying the transaction in the context (the
   cqrs-eda patterns, section 5a, show one). In CQRS mode use its
   `WithUnitOfWork` middleware. Never pass `*sql.Tx` through the core.
9. **Other contexts and services are just more ports.** To use another
   bounded context or service, declare a port in your `app` and implement it
   in your `infra` (an anti-corruption layer), so their model doesn't leak
   into yours. In CQRS mode, reacting to another context's events follows
   cqrs-eda's policies.
10. **Test each ring on its own.**
    - Domain: plain unit tests, no fakes.
    - Use cases: hand-written fakes of the ports, table-driven, asserting on
      errors and effects.
    - Driven adapters: one contract suite per port in `infra/<port>test`, run
      by every adapter of that port (memory always; postgres against a real
      database, skipped when none is configured). That keeps the in-memory
      fake honest.
    - Remote clients: against `httptest.Server`.
    - Driving adapters: `httptest` (or a gRPC test server) with fake use
      cases, covering decoding and every error mapping.

## 4. Workflows

Read `references/patterns.md` before writing code in any of these. It has a
compiling example of every piece: entity, use cases, fakes, memory and
postgres adapters, a contract suite, a remote client, an HTTP adapter and
`main.go`.

**New service**
1. CQRS mode wanted? Hand over to the `cqrs-eda` skill to install the
   platform; otherwise `scaffold.sh service`.
2. `scaffold.sh context` for each bounded context.
3. Per context: domain → use cases with fakes and tests → in-memory adapter
   plus contract suite → real driven adapters → driving adapter → wire in
   `main.go`. Run `go vet ./... && go test ./...`.

**New bounded context**: `scaffold.sh context`, build it as above, wire it in
the existing `main.go`.

**Add a driving adapter** (gRPC, CLI, message consumer, scheduled job)
1. New `infra/<tech>` package; declare interfaces for the use cases it calls
   (CQRS mode: take the buses).
2. Map every domain error the use cases can return, in one function.
3. Consumers: acknowledge only after the use case succeeds, and make sure the
   use case tolerates redelivery.
4. Wire it in `main.go` with the same use-case values the other adapters use.
5. Test it with fake use cases.

**Add or replace a driven adapter**
1. Start from the port. If none exists, define it from what the core needs.
2. Write or extend the contract suite, and make the in-memory adapter pass it.
3. Write the real adapter against the same suite.
4. Swap it in `main.go`. Nothing in `domain` or `app` should change; if it
   has to, the port was leaking the old technology, so fix the port first.

**Put an existing dependency behind a port** (applying hexagonal to code that
calls SQL, HTTP or an SDK from its core)
1. Find the leaks:
   ```bash
   go list -e -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}' ./... \
     | awk '$1 ~ /\/(domain|app)(\/|$)/' \
     | grep -E ' (database/sql|net/http|google\.golang\.org/grpc|[^ ]*/infra(/|$)|github\.com/(jackc|aws|redis|segmentio|IBM)/)'
   ```
   Each line is a core package followed by its imports; extend the pattern
   with the project's drivers and SDKs. Also grep the core for calls hidden
   behind helpers (`os.Getenv`, `time.Now`, `http.DefaultClient`).
2. For one dependency at a time: declare the port in the core's language,
   move the code that uses the dependency into `infra/<tech>` as an adapter,
   translate its errors, inject it in `main.go`, add a fake and tests.
   Keep everything compiling after each step.

## 5. Reviewing structure

Check, with `file:line`, what goes wrong and a concrete fix, in plain words
(the user hasn't read this skill, so don't cite rule numbers):
`domain`/`app` importing drivers, transport packages or `infra`; ports with
technology types or driver errors in their signatures; adapters holding
business rules or calling repositories directly instead of a use case; domain
errors not mapped (or mapped differently per route); unexpected errors logged
twice; config read outside `cmd`; package-level state or `init()` wiring;
stores holding entity pointers; adapters with no compile-time port check or no
contract test. Also read `cmd/*/main.go`: wiring bugs live there. In CQRS mode,
delegate the core (commands, queries, events, middleware) to the
`cqrs-reviewer` agent.
