#!/usr/bin/env bash
# Copyright 2026 go-dataspace
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# Sync the Go version pinned in go.mod and golang base image tags to the latest
# published stable Go 1.x release.
#
# Usage: sync-go-version.sh <apply|check> <file>...
#   apply  rewrite every out-of-sync file (default when run via `just`)
#   check  list out-of-sync files and exit 1 without writing
#
# The version is sourced from go.dev (the canonical release feed) and gated on
# the matching golang base image being published on Docker Hub: the image lags
# Go releases, and go.mod must never name a version with no pullable image.
#
# Functions are defined unconditionally; main() runs only on direct execution
# (see the BASH_SOURCE guard), so scripts/tests/sync-go-version.test.sh can
# source this file and unit-test every path with the network calls stubbed.

readonly GO_RELEASES_URL='https://go.dev/dl/?mode=json'
readonly GOLANG_TAGS_URL='https://hub.docker.com/v2/namespaces/library/repositories/golang/tags'

# Succeed iff curl is on PATH. Isolated so tests can stub it.
have_curl() { command -v curl >/dev/null 2>&1; }

# Fetch the go.dev release feed (stable releases only). Isolated for test stubs.
fetch_go_releases() {
    curl -fsSL --max-time 10 "$GO_RELEASES_URL"
}

# Succeed iff a golang base image is published for version $1. The single-tag
# endpoint is the only Docker Hub query documented to filter by tag. Test-stubbed.
image_published() {
    curl -fs --head --max-time 10 -o /dev/null "${GOLANG_TAGS_URL}/$1"
}

# Read a go.dev release feed on stdin; print the highest stable Go 1.x version
# (e.g. "1.26.3"), or nothing if none can be parsed. The regex doubles as input
# validation: the result can only ever be digits and dots.
parse_latest_go_version() {
    grep -oE 'go1\.[0-9]+\.[0-9]+' | sort -V | tail -n1 | sed 's/^go//'
}

# Print the extended-regexp that proves file $1 already pins version $2. The
# trailing boundary stops "1.26.3" from matching a longer tag like "1.26.30".
version_needle() {
    local escaped=${2//./\\.}
    printf 'go(lang)?:? ?%s' "$escaped"
}

# Rewrite the Go version pinned in file $1 to version $2.
apply_version() {
    case "$1" in
        go.mod | */go.mod) sed -i.bak -E "s|^go [0-9]+(\.[0-9]+){0,2}|go ${2}|" "$1" ;;
        release.yaml | */release.yaml) sed -i.bak -E "s|go[0-9]+(\.[0-9]+){0,2}|go${2}|" "$1" ;;
        *)                 sed -i.bak -E "s|golang:[0-9]+(\.[0-9]+){0,2}|golang:${2}|" "$1" ;;
    esac
    rm -f "${1}.bak"
}

# Print each file in $2.. that does not already pin version $1.
find_drift() {
    local version=$1 file
    shift
    for file in "$@"; do
        grep -qP "$(version_needle "$file" "$version")" "$file" || printf '%s\n' "$file"
    done
}

# Resolve the version to sync to: the latest stable Go 1.x that also has a
# published base image. Print it on stdout, or print why not on stderr and
# return 1 (release feed unreachable, or image not on Docker Hub yet).
resolve_version() {
    local version
    version=$(fetch_go_releases 2>/dev/null | parse_latest_go_version) || true
    if [[ -z $version ]]; then
        echo "sync-go-version: could not fetch the latest Go release from go.dev (offline?); skipping" >&2
        return 1
    fi
    if ! image_published "$version"; then
        echo "sync-go-version: Go ${version} released but golang:${version} image is not on Docker Hub yet; skipping" >&2
        return 1
    fi
    printf '%s\n' "$version"
}

main() {
    set -euo pipefail

    if [[ $# -lt 1 ]]; then
        echo "usage: sync-go-version.sh <apply|check> <file>..." >&2
        return 2
    fi
    local mode=$1
    shift
    case "$mode" in
        apply | check) ;;
        *) echo "usage: sync-go-version.sh <apply|check> <file>..." >&2; return 2 ;;
    esac
    if [[ $# -lt 1 ]]; then
        echo "sync-go-version: no files given" >&2
        return 2
    fi
    local file
    for file in "$@"; do
        [[ -f $file ]] || { echo "sync-go-version: no such file: ${file}" >&2; return 2; }
    done

    # A missing curl is a misconfiguration, not a transient failure: fail check
    # loudly (CI must surface it) but let apply -- run by the pre-commit hook --
    # skip with a warning so a developer without curl is never blocked.
    if ! have_curl; then
        if [[ $mode == check ]]; then
            echo "sync-go-version: curl is required but not installed" >&2
            return 1
        fi
        echo "sync-go-version: curl is not installed; skipping (install curl to keep Go versions in sync)" >&2
        return 0
    fi

    local version
    version=$(resolve_version) || return 0

    local drift=()
    while IFS= read -r file; do
        drift+=("$file")
    done < <(find_drift "$version" "$@")

    if [[ ${#drift[@]} -eq 0 ]]; then
        return 0
    fi

    echo "sync-go-version: latest published Go 1.x is ${version}; out of sync:" >&2
    printf '  %s\n' "${drift[@]}" >&2

    if [[ $mode == check ]]; then
        echo "run 'just sync-go-version' to fix." >&2
        return 1
    fi

    for file in "${drift[@]}"; do
        apply_version "$file" "$version"
    done
    echo "sync-go-version: synced ${#drift[@]} file(s) to Go ${version}." >&2
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
