# Copyright 2024 go-dataspace
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

BINARY_NAME := "run-dsp"

# Files carrying the Go version that `sync-go-version` keeps in sync with the latest release
GO_VERSION_FILES := "go.mod Dockerfile Dockerfile.debug .woodpecker/build.yaml .woodpecker/lint.yaml .woodpecker/test.yaml .woodpecker/vulncheck.yaml .woodpecker/release.yaml"

default: vulncheck lint test build

# Build run-dsp to _build/run-dsp
[group('go')]
build: (_build "" "")

# Build run-dsp binary with debug symbols to _build/run-dsp.debug
[group('go')]
debug: (_build "-gcflags=all=\"-N -l\"" ".debug")

# Run run-dsp tests
[group('go')]
test: _download_mods
    go test -v ./...

# Lint go code
[group('go')]
lint: _download_mods
    go tool golangci-lint run

# Check for vulnerable libraries
[group('go')]
vulncheck: _download_mods
    go tool govulncheck ./...

# Regenerate code based on directives.
[group('go')]
generate: _download_mods
    go generate ./...

# Generate mock dependencies
[group('go')]
mocks: _download_mods
    go tool mockery

# Install pre-commit git hooks (one-time setup per clone)
[group('dev')]
install-git-hooks:
    @command -v pre-commit >/dev/null || { echo "error: pre-commit not found. Please install pre-commit." >&2; exit 1; }
    pre-commit install --hook-type pre-commit --hook-type pre-push

# Sync go.mod and golang base image tags (Dockerfile/.woodpecker) to the latest published Go 1.x
[group('go')]
sync-go-version mode="apply":
    scripts/sync-go-version.sh {{mode}} {{GO_VERSION_FILES}}

# Run shell-script unit tests
[group('dev')]
test-scripts:
    scripts/tests/sync-go-version.test.sh

_download_mods:
    go mod download

_build gcflags bin_suffix: _download_mods
    - mkdir _build
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build {{gcflags}} -ldflags="-extldflags=-static" -o _build/{{BINARY_NAME}}{{bin_suffix}} ./cmd/

