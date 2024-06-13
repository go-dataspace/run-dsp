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

import (
	"io"
	"net/http"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/internal/dspstatemachine"
	"github.com/go-dataspace/run-dsp/jsonld"
	"github.com/go-dataspace/run-dsp/logging"
)

func getContractNegoReq() shared.ContractNegotiation {
	return shared.ContractNegotiation{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:ContractNegotiation",
		ProviderPID: "urn:uuid:dcbf434c-eacf-4582-9a02-f8dd50120fd3",
		ConsumerPID: "urn:uuid:32541fe6-c580-409e-85a8-8a9a32fbe833",
		State:       "REQUESTED",
	}
}

func getContractNegOff() shared.ContractNegotiation {
	return shared.ContractNegotiation{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:ContractNegotiation",
		ProviderPID: "urn:uuid:dcbf434c-eacf-4582-9a02-f8dd50120fd3",
		ConsumerPID: "urn:uuid:32541fe6-c580-409e-85a8-8a9a32fbe833",
		State:       "OFFERED",
	}
}

func providerContractStateHandler(w http.ResponseWriter, req *http.Request) {
	providerPID := req.PathValue("providerPID")
	if providerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing provider PID")
		return
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, getContractNegoReq())
}

func providerContractRequestHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	contractReq, err := unmarshalAndValidate(req.Context(), reqBody, shared.ContractRequestMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	logger.Debug("Got contract request", "req", contractReq)

	contractArgs := dspstatemachine.ContractArgs{
		BaseArgs:         dspstatemachine.BaseArgs{},
		NegotiationState: 0,
		MessageType:      0,
		StateStorage:     dspstatemachine.GetStateStorage(),
	}
	contractNegotiation, err := dspstatemachine.ProviderCheckContractRequestMessage(req.Context(), contractArgs)
	if err != nil {
		returnError(w, http.StatusInternalServerError, err.Error())
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, contractNegotiation)
}

func providerContractSpecificRequestHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	providerPID := req.PathValue("providerPID")
	if providerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing provider PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	contractReq, err := unmarshalAndValidate(req.Context(), reqBody, shared.ContractRequestMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	logger.Debug("Got contract request", "req", contractReq)
	if providerPID != contractReq.ProviderPID {
		returnError(w, http.StatusBadRequest, "Query providerPID does not match the body one")
		return
	}

	// This just returns a 200 if properly processed according to the spec.
	w.WriteHeader(http.StatusOK)
}

func providerContractEventHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	providerPID := req.PathValue("providerPID")
	if providerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing provider PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	event, err := unmarshalAndValidate(req.Context(), reqBody, shared.ContractNegotiationEventMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got contract event", "event", event)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

func providerContractVerificationHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	providerPID := req.PathValue("providerPID")
	if providerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing provider PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	verification, err := unmarshalAndValidate(req.Context(), reqBody, shared.ContractAgreementVerificationMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got contract verification", "verification", verification)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

func providerContractTerminationHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	providerPID := req.PathValue("providerPID")
	if providerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing provider PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	verification, err := unmarshalAndValidate(req.Context(), reqBody, shared.ContractNegotiationTerminationMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got contract termination", "termination", verification)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

func consumerContractOfferHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	contractOffer, err := unmarshalAndValidate(req.Context(), reqBody, shared.ContractOfferMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	logger.Debug("Got contract offer", "offer", contractOffer)

	validateMarshalAndReturn(req.Context(), w, http.StatusCreated, getContractNegOff())
}

func consumerContractSpecificOfferHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	consumerPID := req.PathValue("consumerPID")
	if consumerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing consumer PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	offer, err := unmarshalAndValidate(req.Context(), reqBody, shared.ContractOfferMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got contract offer", "offer", offer)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

func consumerContractAgreementHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	consumerPID := req.PathValue("consumerPID")
	if consumerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing consumer PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	agreement, err := unmarshalAndValidate(req.Context(), reqBody, shared.ContractAgreementMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got contract agreement", "agreement", agreement)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

func consumerContractEventHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	consumerPID := req.PathValue("consumerPID")
	if consumerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing consumer PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}

	// This should have the event FINALIZED
	event, err := unmarshalAndValidate(req.Context(), reqBody, shared.ContractNegotiationEventMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got contract event", "offer", event)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

func consumerContractTerminationHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	consumerPID := req.PathValue("consumerPID")
	if consumerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing consumer PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}

	// This should have the event FINALIZED
	termination, err := unmarshalAndValidate(req.Context(), reqBody, shared.ContractNegotiationTerminationMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got contract event", "termination", termination)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}
