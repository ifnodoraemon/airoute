#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_dir"
base_version=$(sed -n 's/^[[:space:]]*"version": "\([^"]*\)",*$/\1/p' web/package.json)
test -n "$base_version"
go_base_version=$(sed -n 's/^const BaseVersion = "\([^"]*\)"$/\1/p' internal/buildinfo/version.go)
if [ "$base_version" != "$go_base_version" ]; then
    echo "Go and frontend base versions disagree" >&2
    exit 1
fi

if [ "${GITHUB_REF_TYPE:-}" = tag ]; then
    release_version=${GITHUB_REF_NAME#v}
    case "$release_version" in
        "$base_version") ;;
        "$base_version"-rc.*)
            candidate_number=${release_version#"$base_version"-rc.}
            case "$candidate_number" in
                ''|0*|*[!0-9]*) echo "Invalid release candidate number" >&2; exit 1 ;;
            esac
            ;;
        *) echo "Release tag and base version disagree" >&2; exit 1 ;;
    esac
    printf '%s\n' "$release_version"
    exit 0
fi

revision=$(git rev-parse --short HEAD 2>/dev/null || printf unknown)
dirty_suffix=
if ! git diff --quiet HEAD -- 2>/dev/null; then
    dirty_suffix=.dirty
fi
exact_tag=$(git describe --tags --exact-match HEAD 2>/dev/null || true)
if [ "$exact_tag" = "v$base_version" ] && [ -z "$dirty_suffix" ]; then
    printf '%s\n' "$base_version"
else
    printf '%s-dev+g%s%s\n' "$base_version" "$revision" "$dirty_suffix"
fi
