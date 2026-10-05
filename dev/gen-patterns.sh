#!/usr/bin/env bash
# Regenerates skills/hexagonal/references/patterns.md from dev/sample-library,
# after checking that the sample compiles and its tests pass.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
src="$root/dev/sample-library"
out="$root/skills/hexagonal/references/patterns.md"

(cd "$src" && test -z "$(gofmt -l .)" && go vet ./... && go test ./... >/dev/null)

# sec <title> <intro> <file>...
sec() {
	printf '\n## %s\n\n%s\n' "$1" "$2"
	shift 2
	for f in "$@"; do
		printf '\n`%s`\n\n```go\n' "$f"
		cat "$src/$f"
		printf '```\n'
	done
}
{
cat <<'HDR'
# Hexagonal patterns — worked example

A complete, compiling service (module `github.com/acme/library`, Go 1.27) with
one bounded context, `lending`, built without the CQRS platform. Copy the shape,
not the names. Every snippet was verified with `go vet` and `go test`. On
Go < 1.27 the import is `github.com/google/uuid`; write `uuid.Must(uuid.NewV7())`
for `uuid.NewV7()` and `uuid.Nil` for `uuid.Nil()`.

With the CQRS platform (`internal/platform/cqrs`), use cases are commands and
queries: take sections 1–4 from the cqrs-eda skill's patterns instead, and keep
sections 5–10 here, except that driving adapters call `cqrs.Send`/`cqrs.Ask`
instead of use-case interfaces.

## Contents

1. Domain: entity, domain errors, repository port
2. Write use cases, and a driven port to another system
3. Read use case and read port
4. Testing a use case with fakes
5. Driven adapter: in-memory storage
6. Driven adapter: PostgreSQL
7. Contract test shared by every storage adapter
8. Driven adapter: remote service (anti-corruption layer)
9. Driving adapter: HTTP
10. Composition root (`cmd/<service>/main.go`)
HDR
sec "1. Domain: entity, domain errors, repository port" \
	"Pure Go. The domain declares the ports it needs in its own language." \
	internal/lending/domain/book.go
sec "2. Write use cases, and a driven port to another system" \
	"One type per use case with a \`Handle\` method: the driving ports. Ports that only use cases need (other systems, read models) are declared in \`app\`." \
	internal/lending/app/lend_book.go internal/lending/app/add_book.go
sec "3. Read use case and read port" \
	"Reads return views, never entities." \
	internal/lending/app/get_book.go
sec "4. Testing a use case with fakes" \
	"Hand-written fakes of the driven ports, table-driven, asserting on errors and effects." \
	internal/lending/app/lend_book_test.go
sec "5. Driven adapter: in-memory storage" \
	"Stores records, not entities, and rebuilds the entity on load. Compile-time assertions prove it fits the ports." \
	internal/lending/infra/memory/books.go
sec "6. Driven adapter: PostgreSQL" \
	"SQL, NULLs and driver errors stop here; callers get domain values and \`domain.ErrNotFound\`. Register the driver (e.g. \`_ \"github.com/jackc/pgx/v5/stdlib\"\`) in \`main\`." \
	internal/lending/infra/postgres/books.go
sec "7. Contract test shared by every storage adapter" \
	"One suite per port; each adapter's test calls it. For postgres, run the same \`booktest.Run\` against a real database (testcontainers-go, or skip when no database URL is set)." \
	internal/lending/infra/booktest/contract.go internal/lending/infra/memory/books_test.go
sec "8. Driven adapter: remote service (anti-corruption layer)" \
	"The remote API's paths, payloads and status codes stop here. Test it against \`httptest.Server\`." \
	internal/lending/infra/membersapi/client.go internal/lending/infra/membersapi/client_test.go
sec "9. Driving adapter: HTTP" \
	"Decode → use case → map errors → encode. The adapter declares the use cases it needs as small interfaces, owns its JSON shapes, and maps every domain error in one place." \
	internal/lending/infra/httpapi/handlers.go internal/lending/infra/httpapi/handlers_test.go
sec "10. Composition root" \
	"The only place that knows the concrete adapters. Config is read here and passed down as values." \
	cmd/library/main.go
} > "$out"
echo "wrote $out"
