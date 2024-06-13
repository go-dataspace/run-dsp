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
	"strings"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/internal/dspstatemachine"
	"github.com/go-dataspace/run-dsp/jsonld"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/google/uuid"
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
	contractReq, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.ContractRequestMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	logger.Debug("Got contract request", "req", contractReq)

	parts := strings.Split(contractReq.ConsumerPID, ":")
	uuidPart := parts[len(parts)-1]
	consumerPID, err := uuid.Parse(uuidPart)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: ConsumerPID is not a UUID")
		return
	}

	contractArgs := dspstatemachine.ContractArgs{
		BaseArgs: dspstatemachine.BaseArgs{
			ParticipantRole:         dspstatemachine.Provider,
			ConsumerProcessId:       consumerPID,
			ConsumerCallbackAddress: contractReq.CallbackAddress,
		},
		StateStorage: dspstatemachine.GetStateStorage(req.Context()),
	}
	contractNegotiation, err := dspstatemachine.ProviderCheckContractRequestMessage(req.Context(), contractArgs)
	if err != nil {
		returnError(w, http.StatusInternalServerError, err.Error())
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, contractNegotiation)
	channel := dspstatemachine.ExtractChannel(req.Context())
	channel <- dspstatemachine.StateStorageChannelMessage{
		Context:   req.Context(),
		ProcessID: consumerPID,
	}
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
	contractReq, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.ContractRequestMessage{})
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
	event, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.ContractNegotiationEventMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got contract event", "event", event)

	parts := strings.Split(event.ConsumerPID, ":")
	uuidPart := parts[len(parts)-1]
	consumerPID, err := uuid.Parse(uuidPart)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: ConsumerPID is not a UUID")
		return
	}
	parts = strings.Split(event.ProviderPID, ":")
	uuidPart = parts[len(parts)-1]

	if providerPID != uuidPart {
		returnError(w, http.StatusBadRequest, "Invalid request: ProviderPID in message does not match path parameter")
		return
	}

	uuidProviderPID, err := uuid.Parse(uuidPart)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: ProviderPID is not a UUID")
		return
	}

	if event.EventType != "dspace:ACCEPTED" {
		returnError(w, http.StatusBadRequest, "Invalid request: Event type not 'dspace:ACCEPTED'")
		return
	}

	contractArgs := dspstatemachine.ContractArgs{
		BaseArgs: dspstatemachine.BaseArgs{
			ParticipantRole:   dspstatemachine.Provider,
			ConsumerProcessId: consumerPID,
			ProviderProcessId: uuidProviderPID,
		},
		NegotiationState: dspstatemachine.Accepted,
		StateStorage:     dspstatemachine.GetStateStorage(req.Context()),
	}
	contractNegotiation, err := dspstatemachine.ProviderCheckContractAcceptedMessage(req.Context(), contractArgs)
	if err != nil {
		returnError(w, http.StatusInternalServerError, err.Error())
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, contractNegotiation)
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
	verification, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.ContractAgreementVerificationMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got contract verification", "verification", verification)

	parts := strings.Split(verification.ConsumerPID, ":")
	uuidPart := parts[len(parts)-1]
	consumerPID, err := uuid.Parse(uuidPart)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: ConsumerPID is not a UUID")
		return
	}
	parts = strings.Split(verification.ProviderPID, ":")
	uuidPart = parts[len(parts)-1]

	if providerPID != uuidPart {
		returnError(w, http.StatusBadRequest, "Invalid request: ProviderPID in message does not match path parameter")
		return
	}

	uuidProviderPID, err := uuid.Parse(uuidPart)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: ProviderPID is not a UUID")
		return
	}

	contractArgs := dspstatemachine.ContractArgs{
		BaseArgs: dspstatemachine.BaseArgs{
			ParticipantRole:   dspstatemachine.Provider,
			ConsumerProcessId: consumerPID,
			ProviderProcessId: uuidProviderPID,
		},
		NegotiationState: dspstatemachine.Accepted,
		StateStorage:     dspstatemachine.GetStateStorage(req.Context()),
	}
	contractNegotiation, err := dspstatemachine.ProviderCheckContractAgreeementVerificationMessage(
		req.Context(), contractArgs)
	if err != nil {
		returnError(w, http.StatusInternalServerError, err.Error())
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, contractNegotiation)
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
	verification, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.ContractNegotiationTerminationMessage{})
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
	contractOffer, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.ContractOfferMessage{})
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
	offer, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.ContractOfferMessage{})
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
	agreement, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.ContractAgreementMessage{})
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
	event, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.ContractNegotiationEventMessage{})
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
	termination, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.ContractNegotiationTerminationMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got contract event", "termination", termination)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}
