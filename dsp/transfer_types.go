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

package dsp

import "github.com/go-dataspace/run-dsp/jsonld"

// TransferRequestMessage requests a data transfer.
type TransferRequestMessage struct {
	Context         jsonld.Context `json:"@context,omitempty"`
	Type            string         `json:"@type,omitempty" validate:"required,eq=dspace:TransferRequestMessage"`
	AgreementID     string         `json:"dspace:agreementID" validate:"required"`
	Format          string         `json:"dct:format" validate:"required"`
	DataAddress     DataAddress    `json:"dspace:dataAddress"`
	CallbackAddress string         `json:"dspace:callbackAddress" validate:"required"`
	ConsumerPID     string         `json:"dspace:consumerPid" validate:"required"`
}

// TransferStartMessage signals a transfer start.
type TransferStartMessage struct {
	Context     jsonld.Context `json:"@context,omitempty"`
	Type        string         `json:"@type,omitempty" validate:"required,eq=dspace:TransferStartMessage"`
	ProviderPID string         `json:"dspace:providerPid" validate:"required"`
	ConsumerPID string         `json:"dspace:consumerPid" validate:"required"`
	DataAddress DataAddress    `json:"dspace:dataAddress"`
}

// TransferSuspensionMessage signals the suspension of a datatransfer.
type TransferSuspensionMessage struct {
	Context     jsonld.Context   `json:"@context,omitempty"`
	Type        string           `json:"@type,omitempty" validate:"required,eq=dspace:TransferSuspensionMessage"`
	ProviderPID string           `json:"dspace:providerPid,omitempty" validate:"required"`
	ConsumerPID string           `json:"dspace:consumerPid,omitempty" validate:"required"`
	Code        string           `json:"code,omitempty"`
	Reason      []map[string]any `json:"reason,omitempty"`
}

// TransferCompletionMessage signals the completion of a datatransfer.
type TransferCompletionMessage struct {
	Context     jsonld.Context `json:"@context,omitempty"`
	Type        string         `json:"@type,omitempty" validate:"required,eq=dspace:TransferCompletionMessage"`
	ProviderPID string         `json:"dspace:providerPid,omitempty" validate:"required"`
	ConsumerPID string         `json:"dspace:consumerPid,omitempty" validate:"required"`
}

// TransferTerminationMessage signals the suspension of a datatransfer.
type TransferTerminationMessage struct {
	Context     jsonld.Context   `json:"@context,omitempty"`
	Type        string           `json:"@type,omitempty" validate:"required,eq=dspace:TransferTerminationMessage"`
	ProviderPID string           `json:"dspace:providerPid,omitempty" validate:"required"`
	ConsumerPID string           `json:"dspace:consumerPid,omitempty" validate:"required"`
	Code        string           `json:"code,omitempty"`
	Reason      []map[string]any `json:"reason,omitempty"`
}

// TransferProcess are state change reponses.
type TransferProcess struct {
	Context     jsonld.Context `json:"@context,omitempty"`
	Type        string         `json:"@type,omitempty" validate:"required,eq=dspace:TransferProcess"`
	ProviderPID string         `json:"dspace:providerPid,omitempty" validate:"required"`
	ConsumerPID string         `json:"dspace:consumerPid,omitempty" validate:"required"`
	State       string         `json:"dspace:state" validate:"required,transfer_state"`
}

// TransferError signals the suspension of a datatransfer.
type TransferError struct {
	Context     jsonld.Context   `json:"@context,omitempty"`
	Type        string           `json:"@type,omitempty" validate:"required,eq=dspace:TransferError"`
	ProviderPID string           `json:"dspace:providerPid,omitempty" validate:"required"`
	ConsumerPID string           `json:"dspace:consumerPid,omitempty" validate:"required"`
	Code        string           `json:"code,omitempty"`
	Reason      []map[string]any `json:"reason,omitempty"`
}

// DataAddress represents a dataspace data address.
type DataAddress struct {
	Type               string             `json:"@type,omitempty" validate:"required,eq=dspace:DataAddress"`
	EndpointType       string             `json:"endpointType" validate:"required"`
	Endpoint           string             `json:"endpoint" validate:"required"`
	EndpointProperties []EndpointProperty `json:"endpointProperties"`
}

// EndpointProperty represents endpoint properties.
type EndpointProperty struct {
	Type  string `json:"@type,omitempty" validate:"required,eq=dspace:EndpointProperty"`
	Name  string `json:"dspace:name" validate:"required"`
	Value string `json:"dspace:value" validate:"required"`
}
