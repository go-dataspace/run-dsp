// Copyright 2024 go-dataspace
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package shared

// This file contains types to support the IDSA dataspace protocol.
// Currently we support 2024-1.
// Reference: https://docs.internationaldataspaces.org/ids-knowledgebase/v/dataspace-protocol

import (
	"go-dataspace.eu/run-dsp/jsonld"
)

// VersionResponse contains multiple protocol version specifications.
type VersionResponse struct {
	Context          jsonld.Context    `json:"@context"`
	ProtocolVersions []ProtocolVersion `json:"protocolVersions" validate:"required,gte=1,dive"`
}

// ProtocolVersion contains a version and the path to the endpoints.
type ProtocolVersion struct {
	Version string `json:"version" validate:"required"`
	Path    string `json:"path" validate:"required"`
}

// Multilanguage is a DCAT multilanguage set.
type Multilanguage struct {
	Value    string `json:"@value" validate:"required"`
	Language string `json:"@language" validate:"required"`
}

// PublishInfo is a simplified struct to store where to download a file.
type PublishInfo struct {
	URL   string
	Token string
}

// DSPError is an amalgamation of all DSP errors combined into one.
type DSPError struct {
	Context     jsonld.Context  `json:"@context"`
	Type        string          `json:"@type"`
	ProviderPID string          `json:"dspace:providerPid,omitempty"`
	ConsumerPID string          `json:"dspace:consumerPid,omitempty"`
	Code        string          `json:"dspace:code,omitempty"`
	Reason      []Multilanguage `json:"dspace:reason,omitempty"`
	Description []Multilanguage `json:"dct:description,omitempty"`
}
