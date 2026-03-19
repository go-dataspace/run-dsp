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
HELM_REGISTRY := "oci://codeberg.org/go-dataspace/helm"

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


# Lint all charts
[group('helm')]
helm-lint:
    - helm lint charts/run-dsp
    - helm lint charts/run-dsp-stack

# Package all charts
[group('helm')]
package-charts:
    helm package charts/run-dsp
    helm package charts/run-dsp-stack

# Publish all charts, specifically made for woodpecker
[group('helm')]
publish-charts: package-charts
    helm push run-dsp-{{ trim_start_match(env("CI_COMMIT_TAG"), "helm-v") }}.tgz {{ HELM_REGISTRY }}
    helm push run-dsp-stack-{{ trim_start_match(env("CI_COMMIT_TAG"), "helm-v") }}.tgz {{ HELM_REGISTRY }}

_download_mods:
    go mod download

_build gcflags bin_suffix: _download_mods
    - mkdir _build
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build {{gcflags}} -ldflags="-extldflags=-static" -o _build/{{BINARY_NAME}}{{bin_suffix}} ./cmd/

