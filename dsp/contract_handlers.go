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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/internal/dspstatemachine"
	"github.com/go-dataspace/run-dsp/jsonld"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/go-dataspace/run-dsp/odrl"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/provider/v1"
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

func (dh *dspHandlers) providerContractStateHandler(w http.ResponseWriter, req *http.Request) {
	providerPID := req.PathValue("providerPID")
	if providerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing provider PID")
		return
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, getContractNegoReq())
}

func (dh *dspHandlers) providerContractRequestHandler(w http.ResponseWriter, req *http.Request) {
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

	parts = strings.Split(contractReq.Offer.Target, ":")
	uuidPart = parts[len(parts)-1]
	targetID, err := uuid.Parse(uuidPart)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: Target ID is not a UUID")
		return
	}

	_, err = dh.provider.GetDataset(req.Context(), &providerv1.GetDatasetRequest{
		DatasetId: targetID.String(),
	})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: Invalid target")
		return
	}

	contractArgs := dspstatemachine.ContractArgs{
		BaseArgs: dspstatemachine.BaseArgs{
			ParticipantRole:         dspstatemachine.Provider,
			ConsumerProcessId:       consumerPID,
			ProviderProcessId:       uuid.New(),
			ConsumerCallbackAddress: contractReq.CallbackAddress,
		},
		Offer:        contractReq.Offer,
		StateStorage: dspstatemachine.GetStateStorage(req.Context()),
	}
	contractNegotiation, err := dspstatemachine.ProviderCheckContractRequestMessage(req.Context(), contractArgs)
	if err != nil {
		returnError(w, http.StatusInternalServerError, err.Error())
		return
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, contractNegotiation)
	go sendUUID(req.Context(), contractArgs.ProviderProcessId, dspstatemachine.Contract)
}

func (dh *dspHandlers) providerContractSpecificRequestHandler(w http.ResponseWriter, req *http.Request) {
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

func (dh *dspHandlers) providerContractEventHandler(w http.ResponseWriter, req *http.Request) {
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
		return
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, contractNegotiation)
}

func (dh *dspHandlers) providerContractVerificationHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	providerPID := req.PathValue("providerPID")
	if providerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing provider PID")
		return
	}
	_, err := uuid.Parse(providerPID)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: provicer PID is not a UUID")
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

	parts := strings.Split(verification.ProviderPID, ":")
	uuidPart := parts[len(parts)-1]

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
			ProviderProcessId: uuidProviderPID,
		},
		NegotiationState: dspstatemachine.Accepted,
		StateStorage:     dspstatemachine.GetStateStorage(req.Context()),
	}
	contractNegotiation, err := dspstatemachine.ProviderCheckContractAgreementVerificationMessage(
		req.Context(), contractArgs)
	if err != nil {
		returnError(w, http.StatusInternalServerError, err.Error())
		return
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, contractNegotiation)
	go sendUUID(req.Context(), contractArgs.ProviderProcessId, dspstatemachine.Contract)
}

func (dh *dspHandlers) providerContractTerminationHandler(w http.ResponseWriter, req *http.Request) {
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
	verification, err := shared.UnmarshalAndValidate(
		req.Context(), reqBody, shared.ContractNegotiationTerminationMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got contract termination", "termination", verification)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

func (dh *dspHandlers) consumerContractOfferHandler(w http.ResponseWriter, req *http.Request) {
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

func (dh *dspHandlers) consumerContractSpecificOfferHandler(w http.ResponseWriter, req *http.Request) {
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

func (dh *dspHandlers) consumerContractAgreementHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	consumerPIDString := req.PathValue("consumerPID")
	if consumerPIDString == "" {
		returnError(w, http.StatusBadRequest, "Missing consumer PID")
		return
	}
	consumerPID, err := uuid.Parse(consumerPIDString)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: consumer PID is not a UUID")
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

	parts := strings.Split(agreement.ProviderPID, ":")
	uuidPart := parts[len(parts)-1]
	providerPID, err := uuid.Parse(uuidPart)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: ProviderPID is not a UUID")
		return
	}

	logger.Debug("Got contract agreement", "agreement", agreement)

	contractArgs := dspstatemachine.ContractArgs{
		BaseArgs: dspstatemachine.BaseArgs{
			ParticipantRole:         dspstatemachine.Provider,
			ConsumerProcessId:       consumerPID,
			ProviderProcessId:       providerPID,
			ProviderCallbackAddress: agreement.CallbackAddress,
		},
		StateStorage: dspstatemachine.GetStateStorage(req.Context()),
		Agreement:    agreement.Agreement,
	}
	contractNegotiation, err := dspstatemachine.ConsumerCheckContractAgreedRequest(req.Context(), contractArgs)
	if err != nil {
		returnError(w, http.StatusInternalServerError, err.Error())
		return
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, contractNegotiation)
	go sendUUID(req.Context(), consumerPID, dspstatemachine.Contract)
}

func (dh *dspHandlers) consumerContractEventHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	consumerPIDString := req.PathValue("consumerPID")
	if consumerPIDString == "" {
		returnError(w, http.StatusBadRequest, "Missing consumer PID")
		return
	}
	consumerPID, err := uuid.Parse(consumerPIDString)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: consumer PID is not a UUID")
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
	parts := strings.Split(event.ProviderPID, ":")
	uuidPart := parts[len(parts)-1]
	providerPID, err := uuid.Parse(uuidPart)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: ProviderPID is not a UUID")
		return
	}
	logger.Debug("Got contract event", "offer", event)
	contractArgs := dspstatemachine.ContractArgs{
		BaseArgs: dspstatemachine.BaseArgs{
			ParticipantRole:   dspstatemachine.Provider,
			ConsumerProcessId: consumerPID,
			ProviderProcessId: providerPID,
		},
		StateStorage: dspstatemachine.GetStateStorage(req.Context()),
	}
	contractNegotiation, err := dspstatemachine.ConsumerCheckContractFinalizedRequest(req.Context(), contractArgs)
	if err != nil {
		returnError(w, http.StatusInternalServerError, err.Error())
		return
	}
	validateMarshalAndReturn(req.Context(), w, http.StatusOK, contractNegotiation)

	// FIXME: This is only for testing purposed
	state, _ := contractArgs.StateStorage.FindContractNegotiationState(req.Context(), consumerPID)
	statePID := uuid.New()
	transferState := dspstatemachine.DSPTransferStateStorage{
		StateID:                 statePID,
		ConsumerPID:             statePID,
		State:                   dspstatemachine.UndefinedTransferState,
		ParticipantRole:         dspstatemachine.Consumer,
		AgreementID:             state.Agreement.ID,
		ConsumerCallbackAddress: "http://localhost:8080/run-dsp/v2024-1/callback",
		ProviderCallbackAddress: "http://localhost:8080/run-dsp/v2024-1",
	}
	//nolint:errcheck
	contractArgs.StateStorage.StoreTransferNegotiationState(req.Context(), statePID, transferState)
	go sendUUID(req.Context(), statePID, dspstatemachine.Transfer)
}

func (dh *dspHandlers) consumerContractTerminationHandler(w http.ResponseWriter, req *http.Request) {
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

func (dh *dspHandlers) triggerConsumerContractRequestHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	consumerPID := uuid.New()

	datasetID, err := uuid.Parse(req.PathValue("datasetID"))
	if err != nil {
		returnError(w, http.StatusBadRequest, "Dataset ID is not a UUID")
		return
	}

	logger.Debug("Got trigger request to start contract negotiation")
	contractState := dspstatemachine.DSPContractStateStorage{
		StateID:                 consumerPID,
		ConsumerPID:             consumerPID,
		State:                   dspstatemachine.UndefinedState,
		ConsumerCallbackAddress: "http://localhost:8080/run-dsp/v2024-1/callback",
		ProviderCallbackAddress: "http://localhost:8080/run-dsp/v2024-1",
		ParticipantRole:         dspstatemachine.Consumer,
		DatasetID:               datasetID,
	}

	stateStorage := dspstatemachine.GetStateStorage(req.Context())
	err = stateStorage.StoreContractNegotiationState(req.Context(), consumerPID, contractState)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Failed to store initial state")
		return
	}

	bytes, err := json.Marshal(struct {
		ConsumerPID uuid.UUID
	}{
		ConsumerPID: consumerPID,
	})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Failed to write body")
		return
	}
	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(bytes))

	go sendUUID(req.Context(), consumerPID, dspstatemachine.Contract)
}

func (dh *dspHandlers) getConsumerContractRequestHandler(w http.ResponseWriter, req *http.Request) {
	consumerPID := uuid.New()
	contractRequest := shared.ContractRequestMessage{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:ContractRequestMessage",
		ConsumerPID: fmt.Sprintf("urn:uuid:%s", consumerPID),
		Offer: odrl.MessageOffer{
			PolicyClass: odrl.PolicyClass{
				AbstractPolicyRule: odrl.AbstractPolicyRule{},
				ID:                 fmt.Sprintf("urn:uuid:%s", uuid.New()),
			},
			Type:   "odrl:Offer",
			Target: fmt.Sprintf("urn:uuid:%s", uuid.New()),
		},
		CallbackAddress: "http://localhost:8080/run-dsp/v2024-1/callback",
	}

	contractState := dspstatemachine.DSPContractStateStorage{
		StateID:                 consumerPID,
		ConsumerPID:             consumerPID,
		State:                   dspstatemachine.UndefinedState,
		ConsumerCallbackAddress: "http://localhost:8080/run-dsp/v2024-1/callback",
		ProviderCallbackAddress: "http://localhost:8080/run-dsp/v2024-1",
		ParticipantRole:         dspstatemachine.Consumer,
	}

	stateStorage := dspstatemachine.GetStateStorage(req.Context())
	err := stateStorage.StoreContractNegotiationState(req.Context(), consumerPID, contractState)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Failed to store initial state")
		return
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, contractRequest)
}

// FIXME: This needs to have a function for sending an offer.
func (dh *dspHandlers) triggerProviderContractOfferRequestHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	providerPID := uuid.New()

	logger.Debug("Got trigger request to start contract negotiation")
	contractState := dspstatemachine.DSPContractStateStorage{
		StateID:                 providerPID,
		ProviderPID:             providerPID,
		State:                   dspstatemachine.UndefinedState,
		ConsumerCallbackAddress: "http://localhost:8080/run-dsp/v2024-1/callback",
		ProviderCallbackAddress: "http://localhost:8080/run-dsp/v2024-1",
		ParticipantRole:         dspstatemachine.Provider,
	}

	stateStorage := dspstatemachine.GetStateStorage(req.Context())
	err := stateStorage.StoreContractNegotiationState(req.Context(), providerPID, contractState)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Failed to store initial state")
		return
	}
	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)

	go sendUUID(req.Context(), providerPID, dspstatemachine.Contract)
}

func sendUUID(ctx context.Context, id uuid.UUID, transactionType dspstatemachine.DSPTransactionType) {
	channel := dspstatemachine.ExtractChannel(ctx)
	channel <- dspstatemachine.StateStorageChannelMessage{
		Context:         ctx,
		ProcessID:       id,
		TransactionType: transactionType,
	}
}
