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

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/google/uuid"
)

func (dh *dspHandlers) providerTransferProcessHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	providerPID, err := uuid.Parse(req.PathValue("providerPID"))
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid provider PID")
		return
	}

	contract, err := dh.store.GetProviderTransfer(req.Context(), providerPID)
	if err != nil {
		returnError(w, http.StatusNotFound, "no transfer request found")
		return
	}
	if err := shared.EncodeValid(w, req, http.StatusOK, contract.GetTransferProcess()); err != nil {
		logger.Error("couldn't serve contract state: %w", "err", err)
	}
}

func (dh *dspHandlers) providerTransferRequestHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	transferReq, err := shared.DecodeValid[shared.TransferRequestMessage](req)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	consumerPID, err := uuid.Parse(transferReq.ConsumerPID)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Consumer PID is not a UUID")
		return
	}

	agreementID, err := uuid.Parse(transferReq.AgreementID)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Agreement ID is not a UUID")
		return
	}

	cbURL, err := url.Parse(transferReq.CallbackAddress)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request: Non-valid callback URL.")
		return
	}

	pState, err := statemachine.NewTransferRequest(
		req.Context(),
		dh.store, dh.provider, dh.reconciler,
		consumerPID, agreementID,
		transferReq.Format,
		cbURL, dh.selfURL,
		statemachine.DataspaceProvider,
		statemachine.TransferRequestStates.TRANSFERINITIAL,
		nil,
	)
	if err != nil {
		returnError(w, http.StatusInternalServerError, "could not create transfer request")
	}

	nextState, err := pState.Recv(req.Context(), transferReq)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	apply, err := nextState.Send(req.Context())
	if err != nil {
		returnError(w, http.StatusInternalServerError, "Not able to progress state")
		return
	}

	if err := shared.EncodeValid(w, req, http.StatusOK, nextState.GetTransferProcess()); err != nil {
		logger.Error("Couldn't serve response", "err", err)
		returnError(w, http.StatusInternalServerError, "Failed to serve response")
	}
	go apply()
}

//nolint:cyclop
func progressTransferState[T any](
	dh *dspHandlers, w http.ResponseWriter, req *http.Request, role statemachine.DataspaceRole,
	rawPID string, autoProgress bool,
) {
	logger := logging.Extract(req.Context())
	pid, err := uuid.Parse(rawPID)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid PID")
		return
	}

	var transfer *statemachine.TransferRequest
	switch role {
	case statemachine.DataspaceConsumer:
		transfer, err = dh.store.GetConsumerTransfer(req.Context(), pid)
	case statemachine.DataspaceProvider:
		transfer, err = dh.store.GetProviderTransfer(req.Context(), pid)
	default:
		panic(fmt.Sprintf("unexpected statemachine.TransferRole: %#v", role))
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

	pState := statemachine.GetTransferRequestNegotiation(dh.store, transfer, dh.provider, dh.reconciler)

	nextState, err := pState.Recv(req.Context(), msg)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	apply := func() {}
	if autoProgress {
		apply, err = nextState.Send(req.Context())
		if err != nil {
			returnError(w, http.StatusInternalServerError, "Not able to progress state")
			return
		}
	}

	if err := shared.EncodeValid(w, req, http.StatusOK, nextState.GetTransferProcess()); err != nil {
		logger.Error("Couldn't serve response", "err", err)
		returnError(w, http.StatusInternalServerError, "Failed to serve response")
	}
	go apply()
}

func (dh *dspHandlers) providerTransferStartHandler(w http.ResponseWriter, req *http.Request) {
	progressTransferState[shared.TransferStartMessage](
		dh, w, req, statemachine.DataspaceProvider, req.PathValue("providerPID"), false,
	)
}

func (dh *dspHandlers) providerTransferCompletionHandler(w http.ResponseWriter, req *http.Request) {
	progressTransferState[shared.TransferCompletionMessage](
		dh, w, req, statemachine.DataspaceProvider, req.PathValue("providerPID"), true,
	)
}

// TODO: Handle termination.
func (dh *dspHandlers) providerTransferTerminationHandler(w http.ResponseWriter, req *http.Request) {
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
	termination, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferTerminationMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got transfer termination", "termination", termination)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

// TODO: Handle suspension.
func (dh *dspHandlers) providerTransferSuspensionHandler(w http.ResponseWriter, req *http.Request) {
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
	suspension, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferSuspensionMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got transfer suspension", "suspension", suspension)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

func (dh *dspHandlers) consumerTransferStartHandler(w http.ResponseWriter, req *http.Request) {
	progressTransferState[shared.TransferStartMessage](
		dh, w, req, statemachine.DataspaceConsumer, req.PathValue("consumerPID"), false,
	)
}

func (dh *dspHandlers) consumerTransferCompletionHandler(w http.ResponseWriter, req *http.Request) {
	progressTransferState[shared.TransferCompletionMessage](
		dh, w, req, statemachine.DataspaceConsumer, req.PathValue("consumerPID"), true,
	)
}

// TODO: Handle termination.
func (dh *dspHandlers) consumerTransferTerminationHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	consumerPID := req.PathValue("providerPID")
	if consumerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing consumer PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	termination, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferTerminationMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got transfer termination", "termination", termination)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

// TODO: Handle suspension.
func (dh *dspHandlers) consumerTransferSuspensionHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	consumerPID := req.PathValue("providerPID")
	if consumerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing consumer PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	suspension, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferSuspensionMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got transfer suspension", "suspension", suspension)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

func (dh *dspHandlers) completeTransferRequestHandler(w http.ResponseWriter, req *http.Request) {
	ctx, logger := logging.InjectLabels(req.Context(), "handler", "triggerConsumerContractRequestHandler")
	req = req.WithContext(ctx)

	pid, err := uuid.Parse(req.PathValue("providerPID"))
	if err != nil {
		returnError(w, http.StatusBadRequest, "Not a PID")
		return
	}

	transfer, err := dh.store.GetProviderTransfer(ctx, pid)
	if err != nil {
		returnError(w, http.StatusNotFound, "No contract found")
		return
	}

	pState := statemachine.GetTransferRequestNegotiation(dh.store, transfer, dh.provider, dh.reconciler)
	apply, err := pState.Send(req.Context())
	if err != nil {
		returnError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := shared.EncodeValid(w, req, http.StatusOK, pState.GetTransferProcess()); err != nil {
		logger.Error("Couldn't serve response", "err", err)
		returnError(w, http.StatusInternalServerError, "Failed to serve response")
	}
	go apply()
}
