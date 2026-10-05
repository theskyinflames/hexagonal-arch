# Status

Claude Code plugin for hexagonal architecture (ports and adapters) in Go,
designed to work alone or alongside
[cqrs-eda](https://github.com/theskyinflames/golang-cqrs-eda).

## How a plugin is created

The flow this plugin follows, the same one cqrs-eda used:

1. **Scope.** Decide what the plugin owns and what it leaves to other plugins.
   Here: cqrs-eda owns the core (commands, queries, events, aggregates), this
   plugin owns layout, ports, adapters, wiring and boundary tests, and both
   share one layout.
2. **Worked example first.** Write a small service under `dev/` that
   compiles and passes its tests. It is the source of truth for the
   conventions, so the skill never teaches code that doesn't build.
3. **Generate the reference.** `dev/gen-patterns.sh` checks the sample (gofmt,
   vet, tests) and turns it into `skills/<name>/references/patterns.md`.
   Change the sample and regenerate; never edit `patterns.md` by hand.
4. **Write the skill.** `skills/<name>/SKILL.md`: a `description` that decides
   when Claude loads it (trigger words, near-misses to skip), then rules with
   their reasons, workflows and a review checklist. Long examples stay in
   `references/` so they load only when needed.
5. **Deterministic helpers.** Work that must come out the same every time
   (scaffolding, installs) goes in `skills/<name>/scripts/` with templates in
   `assets/`. Scripts refuse to overwrite, verify their output (gofmt, vet),
   and run on macOS's bash 3.2.
6. **Manifest.** `.claude-plugin/plugin.json` (name, version, description,
   author, license). Check it with `claude plugin validate .`.
7. **Try it locally.** `claude --plugin-dir .` on a scratch project, before
   anyone installs it.
8. **Evals.** Trigger evals (does the skill load for the right prompts and
   not for near-misses) and quality evals (fixtures plus checks on what
   Claude produces). cqrs-eda's `evals/` has the harness to reuse.
9. **Publish.** Push to GitHub through a PR, then list the plugin in a
   marketplace (`.claude-plugin/marketplace.json` in golang-cqrs-eda, which is
   the `theskyinflames` marketplace). Users install with
   `/plugin marketplace add theskyinflames/golang-cqrs-eda` and
   `/plugin install <name>@theskyinflames`.
10. **Iterate.** Use it in real projects; each eval run or real-world miss
    becomes a fix in the sample, the skill or the scripts.

## Done

- [x] Scope and split with cqrs-eda: same `internal/<context>/{domain,app,infra}`
      layout; in CQRS mode commands and queries are the driving ports; in
      standalone mode use cases are `Handle`-shaped types, so a later move to
      CQRS is mechanical
- [x] v1 scope chosen: skill + scaffolding (architecture test, migration
      workflow and reviewer agent deferred)
- [x] `dev/sample-library`: compiled, tested example (entity, use cases with
      fakes, memory + postgres adapters, contract suite, remote client, HTTP
      adapter, `main.go`)
- [x] `dev/gen-patterns.sh` and the generated `references/patterns.md`
- [x] `skills/hexagonal/SKILL.md`: mode detection, layout, rules with
      rationale, workflows, review checklist
- [x] `scripts/scaffold.sh` (`context`, `service`), tested with macOS bash 3.2:
      refuses overwrites and bad names, output builds and is gofmt-clean
- [x] Leak-detection `go list` command in SKILL.md, checked on a fixture with
      `database/sql` in domain and an infra import in app
- [x] Plugin manifest (`claude plugin validate .` passes), README, MIT license
- [x] Pushed to `theskyinflames/hexagonal-arch`; PR #1 merged into `main`

## Pending

- [ ] Try it locally with `claude --plugin-dir .` on a scratch project, alone
      and with cqrs-eda installed
- [ ] List `hexagonal` in the `theskyinflames` marketplace (golang-cqrs-eda's
      `.claude-plugin/marketplace.json`, github source
      `theskyinflames/hexagonal-arch`), through a PR there
- [ ] Trigger evals, measured together with cqrs-eda: both descriptions mention
      hexagonal and adapters, so check that each loads when it should and that
      core-only prompts don't load this skill
- [ ] Quality evals with fixtures: new standalone service, new adapter in a
      CQRS project, putting `database/sql` behind a port
- [ ] Decide on the root `go.mod`: it has no Go files under it (the sample has
      its own module); keep it or remove it
- [ ] Later: copy-in architecture test, migration workflow for whole projects,
      `hex-reviewer` agent
- [ ] Use the plugin in real projects
