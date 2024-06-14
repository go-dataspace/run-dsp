// Copyright 2024 go-dataspace
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package dsp

import (
	"encoding/json"
	"net/http"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/jsonld"
	"github.com/go-dataspace/run-dsp/odrl"
)

type AllTypes struct {
	ContractRequest            shared.ContractRequestMessage   `json:"contract_request"`
	ContractOffer              shared.ContractOfferMessage     `json:"contract_offer"`
	ContractAgreement          shared.ContractAgreementMessage `json:"contract_agreement"`
	ContractNegotiationMessage shared.ContractNegotiation      `json:"contract_negotiation"`
}

func getAllTypes() AllTypes {
	return AllTypes{
		ContractRequest: shared.ContractRequestMessage{
			Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
			Type:        "dspace:ContractRequestMessage",
			ConsumerPID: "urn:uuid:32541fe6-c580-409e-85a8-8a9a32fbe833",
			Offer: odrl.MessageOffer{
				PolicyClass: odrl.PolicyClass{
					AbstractPolicyRule: odrl.AbstractPolicyRule{},
					ID:                 "9f7f0f36-060e-42c5-a478-d2b50bc4970d",
				},
				Type:   "odrl:Offer",
				Target: "urn:uuid:3dd1add8-4d2d-569e-d634-8394a8836a88",
			},
			CallbackAddress: "http:/localhost/",
		},
		ContractOffer: shared.ContractOfferMessage{
			Context:         jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
			Type:            "",
			ProviderPID:     "urn:uuid:dcbf434c-eacf-4582-9a02-f8dd50120fd3",
			ConsumerPID:     "urn:uuid:32541fe6-c580-409e-85a8-8a9a32fbe833",
			Offer:           odrl.MessageOffer{},
			CallbackAddress: "",
		},
		ContractAgreement: shared.ContractAgreementMessage{},
		ContractNegotiationMessage: shared.ContractNegotiation{
			Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
			Type:        "dspace:ContractNegotiation",
			ProviderPID: "urn:uuid:dcbf434c-eacf-4582-9a02-f8dd50120fd3",
			ConsumerPID: "urn:uuid:32541fe6-c580-409e-85a8-8a9a32fbe833",
			State:       "REQUESTED",
		},
	}
}

func returnAllTypes(w http.ResponseWriter, req *http.Request) {
	respBody, err := json.Marshal(getAllTypes())
	if err != nil {
		returnError(w, http.StatusInternalServerError, "Failed to marshal data")
	}

	returnContent(w, http.StatusOK, string(respBody))
}
