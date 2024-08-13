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

import (
	"github.com/go-dataspace/run-dsp/jsonld"
	"github.com/go-dataspace/run-dsp/odrl"
)

// ContractRequestMessage is a dsp contract request.
type ContractRequestMessage struct {
	Context         jsonld.Context    `json:"@context"`
	Type            string            `json:"@type" validate:"required,eq=dspace:ContractRequestMessage"`
	ProviderPID     string            `json:"dspace:providerPid,omitempty"`
	ConsumerPID     string            `json:"dspace:consumerPid" validate:"required"`
	Offer           odrl.MessageOffer `json:"dspace:offer" validate:"required"`
	CallbackAddress string            `json:"dspace:callbackAddress" validate:"required"`
}

// ContractOfferMessage is a DSP contract offer.
type ContractOfferMessage struct {
	Context         jsonld.Context    `json:"@context"`
	Type            string            `json:"@type" validate:"required,eq=dspace:ContractOfferMessage"`
	ProviderPID     string            `json:"dspace:providerPid" validate:"required"`
	ConsumerPID     string            `json:"dspace:consumerPid"`
	Offer           odrl.MessageOffer `json:"dspace:offer" validate:"required"`
	CallbackAddress string            `json:"dspace:callbackAddress" validate:"required"`
}

// ContractAgreementMessage is a DSP contract agreement.
type ContractAgreementMessage struct {
	Context         jsonld.Context `json:"@context"`
	Type            string         `json:"@type" validate:"required,eq=dspace:ContractAgreementMessage"`
	ProviderPID     string         `json:"dspace:providerPid" validate:"required"`
	ConsumerPID     string         `json:"dspace:consumerPid"`
	Agreement       odrl.Agreement `json:"dspace:agreement" validate:"required"`
	CallbackAddress string         `json:"dspace:callbackAddress" validate:"required"`
}

// ContractAgreementVerificationMessage verifies the contract agreement.
type ContractAgreementVerificationMessage struct {
	Context     jsonld.Context `json:"@context"`
	Type        string         `json:"@type" validate:"required,eq=dspace:ContractAgreementVerificationMessage"`
	ProviderPID string         `json:"dspace:providerPid" validate:"required"`
	ConsumerPID string         `json:"dspace:consumerPid" validate:"required"`
}

// ContractNegotiationEventMessage notifies of a contract event.
type ContractNegotiationEventMessage struct {
	Context     jsonld.Context `json:"@context"`
	Type        string         `json:"@type" validate:"required,eq=dspace:ContractNegotiationEventMessage"`
	ProviderPID string         `json:"dspace:providerPid" validate:"required"`
	ConsumerPID string         `json:"dspace:consumerPid" validate:"required"`
	EventType   string         `json:"dspace:eventType" validate:"required,oneof=dspace:ACCEPTED dspace:FINALIZED"`
}

// ContractNegotiationTerminationMessage terminates the negotiation.
type ContractNegotiationTerminationMessage struct {
	Context     jsonld.Context   `json:"@context"`
	Type        string           `json:"@type" validate:"required,eq=dspace:ContractNegotiationTerminationMessage"`
	ProviderPID string           `json:"dspace:providerPid" validate:"required"`
	ConsumerPID string           `json:"dspace:consumerPid" validate:"required"`
	Code        string           `json:"dspace:code"`
	Reason      []map[string]any `json:"dspace:reason"`
}

// ContractNegotiation is a response to show the state of the contract negotiation.
type ContractNegotiation struct {
	Context     jsonld.Context `json:"@context"`
	Type        string         `json:"@type" validate:"required,eq=dspace:ContractNegotiation"`
	ProviderPID string         `json:"dspace:providerPid" validate:"required"`
	ConsumerPID string         `json:"dspace:consumerPid" validate:"required"`
	State       string         `json:"dspace:state" validate:"required"`
}
