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
	"net/http"
	"time"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/internal/auth"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/internal/dspstatemachine/httpreq"
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

func (h *httpContractService) configureRequest() func(r *http.Request) {
	return func(r *http.Request) {
		r.Header.Add("Authorization", auth.ExtractUserInfo(h.Context).String())
		r.Header.Add("Content-Type", "application/json")
		r.Header.Add("Accept", "application/json")
	}
}

func (h *httpContractService) sendPostRequest(ctx context.Context, url string, reqBody []byte) ([]byte, error) {
	respBody, err := httpreq.New(
		ctx,
		http.MethodPost,
		url,
		httpreq.WithBody(bytes.NewBuffer(reqBody)),
		httpreq.WithRequestConfigure(h.configureRequest()),
		httpreq.WithStatusCodeCheck(statusCodeCheck),
		// httpreq.WithCancel(c.selfDestruct),
	).Do()
	// NOTE: If the URL is incorrect (in my test missing a / for http://) we get context cancelled message
	//       instead of the real one.
	if err != nil {
		return nil, err
	}

	return respBody, nil
}

func (h *httpContractService) SendContractAgreement(ctx context.Context) error {
	logger := logging.Extract(ctx)
	logger.Debug("In SendContractAgreement")
	agreement := shared.ContractAgreementMessage{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:ContractAgreementMessage",
		ProviderPID: fmt.Sprintf("urn:uuid:%s", h.ContractState.ProviderPID),
		ConsumerPID: fmt.Sprintf("urn:uuid:%s", h.ContractState.ConsumerPID),
		Agreement: odrl.Agreement{
			PolicyClass: odrl.PolicyClass{},
			Type:        "odrl:Agreement",
			ID:          fmt.Sprintf("urn:uuid:%s", uuid.New()),
			Target:      fmt.Sprintf("urn:uuid:%s", uuid.New()),
			Timestamp:   time.Now(),
		},
		CallbackAddress: h.ContractState.ProviderCallbackAddress,
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, agreement)
	if err != nil {
		return err
	}

	targetUrl := fmt.Sprintf("%s/negotiations/%s/agreement", h.ContractState.ConsumerCallbackAddress, h.ContractState.ConsumerPID)
	logger.Debug("Sending ContractAgreement", "target_url", targetUrl, "contract_agreement", agreement)
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
		if contractNegotiation.State != "dspace:AGREED" {
			return errors.New("Invalid state returned")
		}
	}

	return nil
}

func statusCodeCheck(i int) bool {
	return i >= 200 && i < 300
}
