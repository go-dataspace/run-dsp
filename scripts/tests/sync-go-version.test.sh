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
# Unit tests for scripts/sync-go-version.sh. Run directly or via `just test-scripts`.
# The script is sourced (not executed) and its environment probes -- fetch_go_releases,
# image_published and have_curl -- are stubbed, so every path runs deterministically.

set -uo pipefail # deliberately not -e: assertions must continue past a failure

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
# shellcheck source-path=SCRIPTDIR
# shellcheck source=../sync-go-version.sh
source "${SCRIPT_DIR}/sync-go-version.sh"

TEST_TMP=$(mktemp -d)
trap 'rm -rf "$TEST_TMP"' EXIT

# --- assertion harness --------------------------------------------------------
tests_run=0
tests_failed=0

ok() {
    tests_run=$((tests_run + 1))
    printf '  ok   - %s\n' "$1"
}
bad() {
    tests_run=$((tests_run + 1))
    tests_failed=$((tests_failed + 1))
    printf '  FAIL - %s\n         %s\n' "$1" "$2"
}
section() { printf '\n# %s\n' "$1"; }

assert_eq() {
    if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1" "want [$2] got [$3]"; fi
}
assert_rc() {
    if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1" "want rc $2 got rc $3"; fi
}
assert_has() { case "$2" in *"$3"*) ok "$1" ;; *) bad "$1" "[$2] has no [$3]" ;; esac; }

# --- environment stubs --------------------------------------------------------
# Behaviour is driven by these globals; each test sets them before invoking.
STUB_FEED='' # body returned by fetch_go_releases; empty => the fetch "fails"
STUB_IMAGE_RC=0 # return code of image_published
STUB_HAVE_CURL=0 # return code of have_curl (0 = curl present)

fetch_go_releases() { [[ -n "$STUB_FEED" ]] && printf '%s' "$STUB_FEED"; }
image_published() { return "$STUB_IMAGE_RC"; }
have_curl() { return "$STUB_HAVE_CURL"; }

# A representative go.dev feed: latest 1.x plus the previous minor line.
FEED='[{"version":"go1.26.3","stable":true},{"version":"go1.25.10","stable":true}]'

# --- fixtures -----------------------------------------------------------------
wd_n=0
workdir() {
    wd_n=$((wd_n + 1))
    local d="$TEST_TMP/wd${wd_n}"
    mkdir -p "$d/.woodpecker"
    printf '%s' "$d"
}
seed() { # dir version -- write go.mod + Dockerfile + woodpecker yaml pinned to $2
    printf 'module example.com/m\n\ngo %s\n' "$2" >"$1/go.mod"
    printf 'FROM docker.io/library/golang:%s AS builder\n' "$2" >"$1/Dockerfile"
    printf 'steps:\n  build:\n    image: golang:%s\n' "$2" >"$1/.woodpecker/build.yaml"
    printf '      GOTOOLCHAIN: "go%s"\n' "$2" >"$1/.woodpecker/release.yaml"
}

# ============================================================================
section "parse_latest_go_version"
assert_eq "picks the latest from a normal feed" \
    "1.26.3" "$(printf '%s' "$FEED" | parse_latest_go_version)"
assert_eq "version sort beats feed order" \
    "1.26.3" "$(printf '%s' '[{"version":"go1.25.10"},{"version":"go1.26.3"}]' | parse_latest_go_version)"
assert_eq "double-digit patch sorts numerically" \
    "1.26.10" "$(printf '%s' '[{"version":"go1.26.3"},{"version":"go1.26.10"}]' | parse_latest_go_version)"
assert_eq "ignores a future Go 2.x release" \
    "1.26.3" "$(printf '%s' '[{"version":"go2.0.0"},{"version":"go1.26.3"}]' | parse_latest_go_version)"
assert_eq "empty feed yields nothing" "" "$(printf '' | parse_latest_go_version)"

# ============================================================================
section "version_needle"
assert_eq "needle is one unified ERE with a tail boundary" \
    'go(lang)?:? ?1\.26\.3([^0-9.]|$)' "$(version_needle 1.26.3)"

# ============================================================================
section "apply_version"
d=$(workdir)
seed "$d" 1.20.0
apply_version "$d/go.mod" 1.26.3
apply_version "$d/Dockerfile" 1.26.3
apply_version "$d/.woodpecker/release.yaml" 1.26.3
assert_eq "rewrites the go.mod directive" "go 1.26.3" "$(grep '^go ' "$d/go.mod")"
assert_has "rewrites the Dockerfile tag" "$(cat "$d/Dockerfile")" "golang:1.26.3"
assert_has "rewrites the release.yaml GOTOOLCHAIN" "$(cat "$d/.woodpecker/release.yaml")" "go1.26.3"
assert_eq "leaves no .bak files behind" "" "$(find "$d" -name '*.bak')"

d=$(workdir)
printf 'module m\n\ngo 1.20\n' >"$d/go.mod"
apply_version "$d/go.mod" 1.26.3
assert_eq "upgrades a 2-component go directive" "go 1.26.3" "$(grep '^go ' "$d/go.mod")"

# ============================================================================
section "find_drift"
d=$(workdir)
seed "$d" 1.26.3
assert_eq "no output when every file is current" \
    "" "$(cd "$d" && find_drift 1.26.3 go.mod Dockerfile .woodpecker/build.yaml)"

d=$(workdir)
seed "$d" 1.20.0
assert_eq "lists every stale file, in argument order" \
    $'go.mod\nDockerfile\n.woodpecker/build.yaml' \
    "$(cd "$d" && find_drift 1.26.3 go.mod Dockerfile .woodpecker/build.yaml)"

d=$(workdir)
seed "$d" 1.26.3
printf 'FROM golang:1.20.0 AS b\n' >"$d/Dockerfile"
assert_eq "lists only the stale file in a mixed set" \
    "Dockerfile" "$(cd "$d" && find_drift 1.26.3 go.mod Dockerfile)"

d=$(workdir)
printf 'module m\n\ngo 1.26.30\n' >"$d/go.mod"
printf 'FROM golang:1.26.30 AS b\n' >"$d/Dockerfile"
printf '   GOTOOLCHAIN: "go1.26.30"\n' > "$d/.woodpecker/release.yaml"
assert_eq "a longer tag (1.26.30) is not mistaken for 1.26.3" \
    $'go.mod\nDockerfile\n.woodpecker/release.yaml' "$(cd "$d" && find_drift 1.26.3 go.mod Dockerfile .woodpecker/release.yaml)"

# ============================================================================
section "resolve_version"
STUB_FEED=$FEED
STUB_IMAGE_RC=0
out=$(resolve_version 2>/dev/null)
rc=$?
assert_eq "resolves to the latest published version" "1.26.3" "$out"
assert_rc "succeeds when feed and image are both ok" 0 "$rc"

STUB_FEED=''
STUB_IMAGE_RC=0
resolve_version >/dev/null 2>&1
rc=$?
assert_rc "fails when the release feed is unreachable" 1 "$rc"

STUB_FEED=$FEED
STUB_IMAGE_RC=1
resolve_version >/dev/null 2>&1
rc=$?
assert_rc "fails when the image is not published" 1 "$rc"

# ============================================================================
section "main: argument validation"
(main) >/dev/null 2>&1
rc=$?
assert_rc "no arguments -> rc 2" 2 "$rc"

(main bogus go.mod) >/dev/null 2>&1
rc=$?
assert_rc "invalid mode -> rc 2" 2 "$rc"

d=$(workdir)
seed "$d" 1.26.3
(cd "$d" && main apply) >/dev/null 2>&1
rc=$?
assert_rc "mode but no files -> rc 2" 2 "$rc"

(cd "$d" && main check go.mod missing.yaml) >/dev/null 2>&1
rc=$?
assert_rc "missing file -> rc 2" 2 "$rc"

# ============================================================================
section "main: check mode"
STUB_FEED=$FEED
STUB_IMAGE_RC=0

d=$(workdir)
seed "$d" 1.26.3
out=$(cd "$d" && main check go.mod Dockerfile .woodpecker/build.yaml 2>&1)
rc=$?
assert_rc "all in sync -> rc 0" 0 "$rc"
assert_eq "all in sync -> no output" "" "$out"

d=$(workdir)
seed "$d" 1.20.0
out=$(cd "$d" && main check go.mod Dockerfile 2>&1)
rc=$?
assert_rc "drift -> rc 1" 1 "$rc"
assert_has "drift -> lists the stale file" "$out" "go.mod"
assert_eq "check never writes" "go 1.20.0" "$(grep '^go ' "$d/go.mod")"

# ============================================================================
section "main: apply mode"
STUB_FEED=$FEED
STUB_IMAGE_RC=0

d=$(workdir)
seed "$d" 1.20.0
(cd "$d" && main apply go.mod Dockerfile .woodpecker/build.yaml) >/dev/null 2>&1
rc=$?
assert_rc "drift -> rc 0" 0 "$rc"
assert_eq "go.mod rewritten" "go 1.26.3" "$(grep '^go ' "$d/go.mod")"
assert_has "Dockerfile rewritten" "$(cat "$d/Dockerfile")" "golang:1.26.3"
assert_eq "no .bak files left behind" "" "$(find "$d" -name '*.bak')"

d=$(workdir)
seed "$d" 1.26.3
(cd "$d" && main apply go.mod Dockerfile) >/dev/null 2>&1
rc=$?
assert_rc "already in sync -> rc 0" 0 "$rc"

# ============================================================================
section "main: network failures skip, never hard-fail"

d=$(workdir)
seed "$d" 1.20.0
STUB_FEED=''
STUB_IMAGE_RC=0
(cd "$d" && main check go.mod Dockerfile) >/dev/null 2>&1
rc=$?
assert_rc "feed unreachable -> rc 0 (skip)" 0 "$rc"
assert_eq "feed unreachable -> no writes" "go 1.20.0" "$(grep '^go ' "$d/go.mod")"

d=$(workdir)
seed "$d" 1.20.0
STUB_FEED=$FEED
STUB_IMAGE_RC=1
out=$(cd "$d" && main apply go.mod Dockerfile 2>&1)
rc=$?
assert_rc "image unpublished -> rc 0 (skip)" 0 "$rc"
assert_eq "image unpublished -> no writes" "go 1.20.0" "$(grep '^go ' "$d/go.mod")"
assert_has "image unpublished -> explains why" "$out" "not on Docker Hub yet"

# ============================================================================
section "main: missing curl"
STUB_FEED=$FEED
STUB_IMAGE_RC=0
STUB_HAVE_CURL=1 # curl absent

d=$(workdir)
seed "$d" 1.20.0
(cd "$d" && main check go.mod) >/dev/null 2>&1
rc=$?
assert_rc "check + no curl -> rc 1 (hard fail)" 1 "$rc"

out=$(cd "$d" && main apply go.mod 2>&1)
rc=$?
assert_rc "apply + no curl -> rc 0 (skip)" 0 "$rc"
assert_has "apply + no curl -> warns" "$out" "curl is not installed"

# ============================================================================
printf '\n%d assertions run, %d failed\n' "$tests_run" "$tests_failed"
[[ $tests_failed -eq 0 ]]
