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

type TransferError struct {
	status   int
	transfer *statemachine.TransferRequest
	dspCode  string
	reason   string
	err      string
}

func (te TransferError) Error() string     { return te.err }
func (te TransferError) StatusCode() int   { return te.status }
func (te TransferError) ErrorType() string { return "dspace:ContractNegotiationError" }
func (te TransferError) DSPCode() string   { return te.dspCode }

func (te TransferError) Description() []shared.Multilanguage { return []shared.Multilanguage{} }

func (te TransferError) Reason() []shared.Multilanguage {
	return []shared.Multilanguage{{Value: te.reason, Language: "en"}}
}

func (te TransferError) ProviderPID() string {
	if te.transfer == nil {
		return ""
	}
	return te.transfer.GetProviderPID().URN()
}

func (te TransferError) ConsumerPID() string {
	if te.transfer == nil {
		return ""
	}
	return te.transfer.GetConsumerPID().URN()
}

func transferError(
	err string, statusCode int, dspCode string, reason string, transfer *statemachine.TransferRequest,
) TransferError {
	return TransferError{
		status:   statusCode,
		transfer: transfer,
		dspCode:  dspCode,
		reason:   reason,
		err:      err,
	}
}

func (dh *dspHandlers) providerTransferProcessHandler(w http.ResponseWriter, req *http.Request) error {
	logger := logging.Extract(req.Context())
	providerPID, err := uuid.Parse(req.PathValue("providerPID"))
	if err != nil {
		return transferError("invalid provider ID", http.StatusBadRequest, "400", "Invalid provider PID", nil)
	}

	contract, err := dh.store.GetProviderTransfer(req.Context(), providerPID)
	if err != nil {
		return contractError(err.Error(), http.StatusNotFound, "404", "TransferRequest not found", nil)
	}
	if err := shared.EncodeValid(w, req, http.StatusOK, contract.GetTransferProcess()); err != nil {
		logger.Error("couldn't serve contract state: %w", "err", err)
	}
	return nil
}

func (dh *dspHandlers) providerTransferRequestHandler(w http.ResponseWriter, req *http.Request) error {
	logger := logging.Extract(req.Context())
	transferReq, err := shared.DecodeValid[shared.TransferRequestMessage](req)
	if err != nil {
		return transferError(fmt.Sprintf("invalid request message: %s", err.Error()),
			http.StatusBadRequest, "400", "Invalid request", nil)
	}

	consumerPID, err := uuid.Parse(transferReq.ConsumerPID)
	if err != nil {
		return transferError(fmt.Sprintf("Invalid consumer ID %s: %s", transferReq.ConsumerPID, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: ConsumerPID is not a UUID", nil)
	}

	agreementID, err := uuid.Parse(transferReq.AgreementID)
	if err != nil {
		return transferError(fmt.Sprintf("Invalid agreement ID %s: %s", transferReq.AgreementID, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: agreement ID is not a UUID", nil)
	}

	cbURL, err := url.Parse(transferReq.CallbackAddress)
	if err != nil {
		return transferError(fmt.Sprintf("Invalid callback URL %s: %s", transferReq.CallbackAddress, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: Non-valid callback URL.", nil)
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
		return transferError(fmt.Sprintf("couldn't create transfer request: %s", err.Error()),
			http.StatusInternalServerError, "500", "Failed to create transfer request", nil)
	}

	nextState, err := pState.Recv(req.Context(), transferReq)
	if err != nil {
		return transferError(
			fmt.Sprintf("couldn't receive message: %s", err.Error()),
			http.StatusBadRequest, "400", "Invalid request", pState.GetTransferRequest())
	}

	apply, err := nextState.Send(req.Context())
	if err != nil {
		return transferError(fmt.Sprintf("couldn't progress to next state: %s", err.Error()),
			http.StatusInternalServerError, "500", "Not able to progress state", nextState.GetTransferRequest())
	}

	if err := shared.EncodeValid(w, req, http.StatusOK, nextState.GetTransferProcess()); err != nil {
		logger.Error("Couldn't serve response", "err", err)
	}
	go apply()
	return nil
}

//nolint:cyclop
func progressTransferState[T any](
	dh *dspHandlers, w http.ResponseWriter, req *http.Request, role statemachine.DataspaceRole,
	rawPID string, autoProgress bool,
) error {
	logger := logging.Extract(req.Context())
	pid, err := uuid.Parse(rawPID)
	if err != nil {
		return transferError(fmt.Sprintf("Invalid PID %s: %s", rawPID, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: PID is not a UUID", nil)
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
		return transferError(fmt.Sprintf("%d transfer request %s not found: %s", role, pid, err),
			http.StatusNotFound, "404", "Transfer request not found", nil)
	}

	msg, err := shared.DecodeValid[T](req)
	if err != nil {
		return transferError(fmt.Sprintf("could not decode message: %s", err),
			http.StatusBadRequest, "400", "Invalid request", transfer)
	}

	logger.Debug("Got contract message", "req", msg)

	pState := statemachine.GetTransferRequestNegotiation(dh.store, transfer, dh.provider, dh.reconciler)

	nextState, err := pState.Recv(req.Context(), msg)
	if err != nil {
		return transferError(fmt.Sprintf("invalid request: %s", err),
			http.StatusBadRequest, "400", "Invalid request", pState.GetTransferRequest())
	}

	apply := func() {}
	if autoProgress {
		apply, err = nextState.Send(req.Context())
		if err != nil {
			return transferError(fmt.Sprintf("couldn't progress to next state: %s", err.Error()),
				http.StatusInternalServerError, "500", "Not able to progress state", nextState.GetTransferRequest())
		}
	}

	if err := shared.EncodeValid(w, req, http.StatusOK, nextState.GetTransferProcess()); err != nil {
		logger.Error("Couldn't serve response", "err", err)
	}

	go apply()

	return nil
}

func (dh *dspHandlers) providerTransferStartHandler(w http.ResponseWriter, req *http.Request) error {
	return progressTransferState[shared.TransferStartMessage](
		dh, w, req, statemachine.DataspaceProvider, req.PathValue("providerPID"), false,
	)
}

func (dh *dspHandlers) providerTransferCompletionHandler(w http.ResponseWriter, req *http.Request) error {
	return progressTransferState[shared.TransferCompletionMessage](
		dh, w, req, statemachine.DataspaceProvider, req.PathValue("providerPID"), true,
	)
}

func (dh *dspHandlers) providerTransferTerminationHandler(w http.ResponseWriter, req *http.Request) error {
	return progressTransferState[shared.TransferTerminationMessage](
		dh, w, req, statemachine.DataspaceProvider, req.PathValue("providerPID"), true,
	)
}

// TODO: Handle suspension.
func (dh *dspHandlers) providerTransferSuspensionHandler(w http.ResponseWriter, req *http.Request) error {
	logger := logging.Extract(req.Context())
	providerPID := req.PathValue("providerPID")
	if providerPID == "" {
		return fmt.Errorf("Missing provider PID")
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("Could not read body")
	}
	suspension, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferSuspensionMessage{})
	if err != nil {
		return fmt.Errorf("Invalid request")
	}

	logger.Debug("Got transfer suspension", "suspension", suspension)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)

	return nil
}

func (dh *dspHandlers) consumerTransferStartHandler(w http.ResponseWriter, req *http.Request) error {
	return progressTransferState[shared.TransferStartMessage](
		dh, w, req, statemachine.DataspaceConsumer, req.PathValue("consumerPID"), false,
	)
}

func (dh *dspHandlers) consumerTransferCompletionHandler(w http.ResponseWriter, req *http.Request) error {
	return progressTransferState[shared.TransferCompletionMessage](
		dh, w, req, statemachine.DataspaceConsumer, req.PathValue("consumerPID"), true,
	)
}

func (dh *dspHandlers) consumerTransferTerminationHandler(w http.ResponseWriter, req *http.Request) error {
	return progressTransferState[shared.TransferTerminationMessage](
		dh, w, req, statemachine.DataspaceConsumer, req.PathValue("consumerPID"), true,
	)
}

// TODO: Handle suspension.
func (dh *dspHandlers) consumerTransferSuspensionHandler(w http.ResponseWriter, req *http.Request) error {
	logger := logging.Extract(req.Context())
	consumerPID := req.PathValue("providerPID")
	if consumerPID == "" {
		return fmt.Errorf("Missing consumner PID")
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	suspension, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferSuspensionMessage{})
	if err != nil {
		return err
	}

	logger.Debug("Got transfer suspension", "suspension", suspension)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
	return nil
}
