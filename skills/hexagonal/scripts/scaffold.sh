#!/usr/bin/env bash
# Scaffolds hexagonal structure in a Go project. Never overwrites anything.
#
# Usage:
#   scaffold.sh context <project-dir> <name>   internal/<name>/{domain,app,infra}
#   scaffold.sh service <project-dir> <name>   cmd/<name>/main.go (composition root)
#
# Names are lowercase Go package names: [a-z][a-z0-9]*.
set -euo pipefail

skill_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmpl="$skill_dir/assets/templates"

usage() { sed -n '4,6p' "${BASH_SOURCE[0]}" | sed 's/^# //' >&2; exit 2; }
(($# == 3)) || usage
kind="$1"
name="$3"
project="$(cd "$2" && pwd)"

# --- checks: nothing is written until all pass ---
[[ -f "$project/go.mod" ]] || { echo "error: no go.mod in $project" >&2; exit 1; }
[[ "$name" =~ ^[a-z][a-z0-9]*$ ]] || { echo "error: '$name' is not a lowercase Go package name" >&2; exit 1; }

case "$kind" in
context)
	dst="$project/internal/$name"
	[[ -e "$dst" ]] && { echo "error: $dst already exists; refusing to overwrite" >&2; exit 1; }
	for layer in domain app infra; do
		mkdir -p "$dst/$layer"
		sed "s/__CONTEXT__/$name/g" "$tmpl/context/$layer/doc.go.tmpl" >"$dst/$layer/doc.go"
	done
	pkgs="./internal/$name/..."
	;;
service)
	dst="$project/cmd/$name"
	[[ -e "$dst" ]] && { echo "error: $dst already exists; refusing to overwrite" >&2; exit 1; }
	mkdir -p "$dst"
	sed "s/__SERVICE__/$name/g" "$tmpl/service/main.go.tmpl" >"$dst/main.go"
	pkgs="./cmd/$name"
	;;
*) usage ;;
esac

# --- verify ---
cd "$project"
gofmt -l "$dst" | (! grep .) || { echo "error: generated code is not gofmt-clean" >&2; exit 1; }
go vet "$pkgs"
echo "created $dst"
