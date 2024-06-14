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

package dspstatemachine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/internal/auth"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/jsonld"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/go-dataspace/run-dsp/odrl"
	"github.com/google/uuid"
)

type httpContractService struct {
	Context       context.Context
	ContractState DSPContractStateStorage
}

func getHttpContractService(ctx context.Context, contractState DSPContractStateStorage) *httpContractService {
	return &httpContractService{
		Context:       ctx,
		ContractState: contractState,
	}
}

func (h *httpContractService) configureRequest(r *http.Request) {
	r.Header.Add("Authorization", auth.ExtractUserInfo(h.Context).String())
	r.Header.Add("Content-Type", "application/json")
	r.Header.Add("Accept", "application/json")
}

func (h *httpContractService) sendPostRequest(ctx context.Context, url string, reqBody []byte) ([]byte, error) {
	logger := logging.Extract(ctx)
	logger.Debug("Going to send POST request", "target_url", url)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	h.configureRequest(req)
	client := &http.Client{}
	resp, err := client.Do(req)
	// NOTE: If the URL is incorrect (in my test missing a / for http://) we get context cancelled message
	//       instead of the real one.
	if err != nil {
		return nil, err
	}
	if resp.StatusCode > 299 {
		return nil, fmt.Errorf("Invalid error code: %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func (h *httpContractService) ConsumerSendContractRequest(ctx context.Context) error {
	logger := logging.Extract(ctx)
	logger.Debug("In ConsumerSendContractRequest")

	contractRequest := shared.ContractRequestMessage{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:ContractRequestMessage",
		ConsumerPID: fmt.Sprintf("urn:uuid:%s", h.ContractState.ConsumerPID),
		Offer: odrl.MessageOffer{
			PolicyClass: odrl.PolicyClass{
				AbstractPolicyRule: odrl.AbstractPolicyRule{},
				ID:                 fmt.Sprintf("urn:uuid:%s", uuid.New()),
			},
			Type:   "odrl:Offer",
			Target: fmt.Sprintf("urn:uuid:%s", uuid.New()),
		},
		CallbackAddress: h.ContractState.ConsumerCallbackAddress,
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, contractRequest)
	if err != nil {
		return err
	}

	targetUrl := fmt.Sprintf("%s/negotiations/request", h.ContractState.ProviderCallbackAddress)
	logger.Debug("Sending ContractRequest", "target_url", targetUrl, "contract_request", contractRequest)
	responseBody, err := h.sendPostRequest(ctx, targetUrl, reqBody)
	if err != nil {
		return err
	}

	// This should have the state OFFERED in ACK, if not empty
	if len(responseBody) != 0 {
		contractNegotiation, err := shared.UnmarshalAndValidate(ctx, responseBody, shared.ContractNegotiation{})
		if err != nil {
			return err
		}

		logger.Debug("Got ContractNegotiation", "cloontract_negotiation", contractNegotiation)
		if contractNegotiation.State != "dspace:REQUESTED" {
			return errors.New("Invalid state returned")
		}
	}

	return nil
}

func (h *httpContractService) ConsumerSendAgreementVerificationRequest(ctx context.Context) error {
	logger := logging.Extract(ctx)
	logger.Debug("In ConsumerAgreementVerificationRequest")

	verification := shared.ContractAgreementVerificationMessage{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:ContractAgreementVerificationMessage",
		ProviderPID: fmt.Sprintf("urn:uuid:%s", h.ContractState.ProviderPID),
		ConsumerPID: fmt.Sprintf("urn:uuid:%s", h.ContractState.ConsumerPID),
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, verification)
	if err != nil {
		return err
	}

	targetUrl := fmt.Sprintf("%s/negotiations/%s/agreement/verification",
		h.ContractState.ProviderCallbackAddress, h.ContractState.ProviderPID)
	logger.Debug("Sending ConsumerAgreementVerificationRequest", "target_url", targetUrl, "contract_request", verification)
	responseBody, err := h.sendPostRequest(ctx, targetUrl, reqBody)
	if err != nil {
		return err
	}

	// This should have the state OFFERED in ACK, if not empty
	if len(responseBody) != 0 {
		contractNegotiation, err := shared.UnmarshalAndValidate(ctx, responseBody, shared.ContractNegotiation{})
		if err != nil {
			return err
		}

		logger.Debug("Got ContractNegotiation", "contract_negotiation", contractNegotiation)
		if contractNegotiation.State != "dspace:REQUESTED" {
			return errors.New("Invalid state returned")
		}
	}

	return nil
}

func (h *httpContractService) ProviderSendContractAgreement(ctx context.Context) (odrl.Agreement, error) {
	logger := logging.Extract(ctx)
	logger.Debug("In ProviderSendContractAgreement")
	agreement := odrl.Agreement{
		PolicyClass: odrl.PolicyClass{},
		Type:        "odrl:Agreement",
		ID:          fmt.Sprintf("urn:uuid:%s", uuid.New()),
		Target:      h.ContractState.Offer.Target,
		Timestamp:   time.Now(),
	}
	agreementMessage := shared.ContractAgreementMessage{
		Context:         jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:            "dspace:ContractAgreementMessage",
		ProviderPID:     fmt.Sprintf("urn:uuid:%s", h.ContractState.ProviderPID),
		ConsumerPID:     fmt.Sprintf("urn:uuid:%s", h.ContractState.ConsumerPID),
		Agreement:       agreement,
		CallbackAddress: h.ContractState.ProviderCallbackAddress,
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, agreementMessage)
	if err != nil {
		return odrl.Agreement{}, err
	}

	targetUrl := fmt.Sprintf("%s/negotiations/%s/agreement",
		h.ContractState.ConsumerCallbackAddress, h.ContractState.ConsumerPID)
	logger.Debug("Sending ContractAgreementMessage", "target_url", targetUrl, "contract_agreement", agreementMessage)
	responseBody, err := h.sendPostRequest(ctx, targetUrl, reqBody)
	if err != nil {
		return odrl.Agreement{}, err
	}

	// This should have the state OFFERED in ACK, if not empty
	if len(responseBody) != 0 {
		contractNegotiation, err := shared.UnmarshalAndValidate(ctx, responseBody, shared.ContractNegotiation{})
		if err != nil {
			return odrl.Agreement{}, err
		}

		logger.Debug("Got ContractNegotiation", "contract_negotiation", contractNegotiation)
		if contractNegotiation.State != "dspace:AGREED" {
			return odrl.Agreement{}, errors.New("Invalid state returned")
		}
	}

	return agreement, nil
}

func (h *httpContractService) ProviderSendContractFinalized(ctx context.Context) error {
	logger := logging.Extract(ctx)
	logger.Debug("In ProviderSendContractFinalized")
	finalized := shared.ContractNegotiationEventMessage{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:ContractNegotiationEventMessage",
		ProviderPID: fmt.Sprintf("urn:uuid:%s", h.ContractState.ProviderPID),
		ConsumerPID: fmt.Sprintf("urn:uuid:%s", h.ContractState.ConsumerPID),
		EventType:   "dspace:FINALIZED",
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, finalized)
	if err != nil {
		return err
	}

	targetUrl := fmt.Sprintf("%s/negotiations/%s/events",
		h.ContractState.ConsumerCallbackAddress, h.ContractState.ConsumerPID)
	logger.Debug("Sending ContractNegotiationEventMessage", "target_url", targetUrl, "contract_agreement", finalized)
	responseBody, err := h.sendPostRequest(ctx, targetUrl, reqBody)
	if err != nil {
		return err
	}

	// This should have the state OFFERED in ACK, if not empty
	if len(responseBody) != 0 {
		contractNegotiation, err := shared.UnmarshalAndValidate(ctx, responseBody, shared.ContractNegotiation{})
		if err != nil {
			return err
		}

		logger.Debug("Got ContractNegotiation", "contract_negotiation", contractNegotiation)
		if contractNegotiation.State != "dspace:FINALIZED" {
			return errors.New("Invalid state returned")
		}
	}

	return nil
}
