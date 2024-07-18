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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/go-dataspace/run-dsp/odrl"
	"github.com/google/uuid"
)

func (dh *dspHandlers) providerContractStateHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	providerPID, err := uuid.Parse(req.PathValue("providerPID"))
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid provider PID")
		return
	}

	contract, err := dh.store.GetProviderContract(req.Context(), providerPID)
	if err != nil {
		returnError(w, http.StatusNotFound, "no contract found")
		return
	}
	if err := shared.EncodeValid(w, req, http.StatusOK, contract.GetContractNegotiation()); err != nil {
		logger.Error("couldn't serve contract state: %w", "error", err)
	}
}

func (dh *dspHandlers) providerContractRequestHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	contractReq, err := shared.DecodeValid[shared.ContractRequestMessage](req)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	logger.Debug("Got contract request", "req", contractReq)

	consumerPID, err := uuid.Parse(contractReq.ConsumerPID)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: ConsumerPID is not a UUID")
		return
	}

	cbURL, err := url.Parse(contractReq.CallbackAddress)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: Non-valid callback URL.")
		return
	}

	// TODO: Maybe make a function in the statemachine that parses the contract request message.
	pState := statemachine.NewContract(
		req.Context(),
		dh.store, dh.provider, dh.client,
		uuid.UUID{}, consumerPID,
		statemachine.ContractStates.INITIAL,
		odrl.Offer{MessageOffer: contractReq.Offer},
		cbURL, dh.selfURL,
		statemachine.ContractProvider,
	)

	nextState, err := pState.Recv(req.Context(), contractReq)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	apply, err := nextState.Send(req.Context())
	if err != nil {
		returnError(w, http.StatusInternalServerError, "Not able to progress state")
		return
	}
	defer apply()

	if err := shared.EncodeValid(w, req, http.StatusOK, nextState.GetContractNegotiation()); err != nil {
		logger.Error("Couldn't serve response", "error", err)
		returnError(w, http.StatusInternalServerError, "Failed to serve response")
	}
}

// TODO: unify this with the other state progression func.
//
//nolint:dupl
func progressContractState[T any](
	dh *dspHandlers, w http.ResponseWriter, req *http.Request, role statemachine.ContractRole, rawPID string,
) {
	logger := logging.Extract(req.Context())
	pid, err := uuid.Parse(rawPID)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid PID")
		return
	}

	var contract *statemachine.Contract
	switch role {
	case statemachine.ContractConsumer:
		contract, err = dh.store.GetConsumerContract(req.Context(), pid)
	case statemachine.ContractProvider:
		contract, err = dh.store.GetProviderContract(req.Context(), pid)
	default:
		panic(fmt.Sprintf("unexpected statemachine.ContractRole: %#v", role))
	}
	if err != nil {
		returnError(w, http.StatusNotFound, "Contract not found")
		return
	}

	msg, err := shared.DecodeValid[T](req)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got contract message", "req", msg)

	pState := statemachine.GetContractNegotiation(dh.store, contract, dh.provider, dh.client)

	nextState, err := pState.Recv(req.Context(), msg)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	apply, err := nextState.Send(req.Context())
	if err != nil {
		returnError(w, http.StatusInternalServerError, "Not able to progress state")
		return
	}
	defer apply()

	if err := shared.EncodeValid(w, req, http.StatusOK, nextState.GetContractNegotiation()); err != nil {
		logger.Error("Couldn't serve response", "error", err)
		returnError(w, http.StatusInternalServerError, "Failed to serve response")
	}
}

func (dh *dspHandlers) providerContractSpecificRequestHandler(w http.ResponseWriter, req *http.Request) {
	progressContractState[shared.ContractRequestMessage](
		dh, w, req, statemachine.ContractProvider, req.PathValue("providerPID"),
	)
}

func (dh *dspHandlers) providerContractEventHandler(w http.ResponseWriter, req *http.Request) {
	progressContractState[shared.ContractNegotiationEventMessage](
		dh, w, req, statemachine.ContractProvider, req.PathValue("providerPID"),
	)
}

func (dh *dspHandlers) providerContractVerificationHandler(w http.ResponseWriter, req *http.Request) {
	progressContractState[shared.ContractAgreementVerificationMessage](
		dh, w, req, statemachine.ContractProvider, req.PathValue("providerPID"),
	)
}

// TODO: Implement terminations.
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
	contractOffer, err := shared.DecodeValid[shared.ContractOfferMessage](req)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	logger.Debug("Got contract offer", "offer", contractOffer)

	providerPID, err := uuid.Parse(contractOffer.ProviderPID)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: ProviderPID is not a UUID")
		return
	}

	cbURL, err := url.Parse(contractOffer.CallbackAddress)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: non-valid callback URL.")
		return
	}

	selfURL, err := url.Parse(dh.selfURL.String())
	if err != nil {
		panic(err.Error())
	}
	selfURL.Path = path.Join(selfURL.Path, "callback")
	cState := statemachine.NewContract(
		req.Context(),
		dh.store, dh.provider, dh.client,
		providerPID, uuid.UUID{},
		statemachine.ContractStates.INITIAL,
		odrl.Offer{MessageOffer: contractOffer.Offer},
		cbURL, selfURL, statemachine.ContractConsumer,
	)

	nextState, err := cState.Recv(req.Context(), contractOffer)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	apply, err := nextState.Send(req.Context())
	if err != nil {
		returnError(w, http.StatusInternalServerError, "Not able to progress state")
		return
	}
	defer apply()

	if err := shared.EncodeValid(w, req, http.StatusOK, nextState.GetContractNegotiation()); err != nil {
		logger.Error("Couldn't serve response", "error", err)
		returnError(w, http.StatusInternalServerError, "Failed to serve response")
	}
}

func (dh *dspHandlers) consumerContractSpecificOfferHandler(w http.ResponseWriter, req *http.Request) {
	progressContractState[shared.ContractOfferMessage](
		dh, w, req, statemachine.ContractConsumer, req.PathValue("consumerPID"),
	)
}

func (dh *dspHandlers) consumerContractAgreementHandler(w http.ResponseWriter, req *http.Request) {
	progressContractState[shared.ContractAgreementMessage](
		dh, w, req, statemachine.ContractConsumer, req.PathValue("consumerPID"),
	)
}

func (dh *dspHandlers) consumerContractEventHandler(w http.ResponseWriter, req *http.Request) {
	progressContractState[shared.ContractNegotiationEventMessage](
		dh, w, req, statemachine.ContractConsumer, req.PathValue("consumerPID"),
	)
}

// TODO: Handle termination in the statemachine.
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

	datasetID, err := uuid.Parse(req.PathValue("datasetID"))
	if err != nil {
		returnError(w, http.StatusBadRequest, "Dataset ID is not a UUID")
		return
	}

	logger.Debug("Got trigger request to start contract negotiation")
	selfURL, err := url.Parse(dh.selfURL.String())
	if err != nil {
		panic(err.Error())
	}
	selfURL.Path = path.Join(selfURL.Path, "callback")
	cInit := statemachine.NewContract(
		req.Context(),
		dh.store, dh.provider, dh.client,
		uuid.UUID{}, uuid.New(),
		statemachine.ContractStates.INITIAL,
		odrl.Offer{
			MessageOffer: odrl.MessageOffer{
				PolicyClass: odrl.PolicyClass{
					AbstractPolicyRule: odrl.AbstractPolicyRule{},
					ID:                 uuid.New().URN(),
				},
				Type:   "odrl:Offer",
				Target: datasetID.URN(),
			},
		},
		dh.selfURL,
		selfURL,
		statemachine.ContractConsumer,
	)
	apply, err := cInit.Send(req.Context())
	if err != nil {
		returnError(w, http.StatusInternalServerError, err.Error())
	}
	defer apply()

	if err := shared.EncodeValid(w, req, http.StatusOK, cInit.GetContractNegotiation()); err != nil {
		logger.Error("Couldn't serve response", "error", err)
		returnError(w, http.StatusInternalServerError, "Failed to serve response")
	}
}
