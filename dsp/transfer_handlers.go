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
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/persistence"
	agreementopts "go-dataspace.eu/run-dsp/dsp/persistence/options/agreement"
	transferopts "go-dataspace.eu/run-dsp/dsp/persistence/options/transfer"
	"go-dataspace.eu/run-dsp/dsp/shared"
	"go-dataspace.eu/run-dsp/dsp/statemachine"
	"go-dataspace.eu/run-dsp/dsp/transfer"
	"go-dataspace.eu/run-dsp/internal/authforwarder"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

type TransferError struct {
	status   int
	transfer *transfer.Request
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
	ctx context.Context,
	err string, statusCode int, dspCode string, reason string, transfer *transfer.Request,
) TransferError {
	ctxslog.Error(
		ctx, "transfer error",
		"statusCode", statusCode,
		"dspCode", dspCode,
		"reason", reason,
		"err", err,
		"role", transfer.GetRole().String(),
		"localPID", transfer.GetLocalPID(),
		"direction", transfer.GetTransferDirection(),
	)
	return TransferError{
		status:   statusCode,
		transfer: transfer,
		dspCode:  dspCode,
		reason:   reason,
		err:      err,
	}
}

func (dh *dspHandlers) providerTransferProcessHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "providerTransferProcessHandler")
	defer span.End()
	ctx = ctxslog.With(ctx, "handler", "providerTransferProcessHandler")
	providerPID, err := uuid.Parse(req.PathValue("providerPID"))
	if err != nil {
		return transferError(ctx, "invalid provider ID", http.StatusBadRequest, "400", "Invalid provider PID", nil)
	}

	contract, err := dh.store.GetTransfer(
		ctx,
		transferopts.WithRolePID(providerPID, constants.DataspaceProvider),
	)
	if err != nil {
		return contractError(ctx, err.Error(), http.StatusNotFound, "404", "TransferRequest not found", nil)
	}
	if err := shared.EncodeValid(w, req, http.StatusOK, contract.GetTransferProcess()); err != nil {
		ctxslog.Err(ctx, "couldn't serve contract state", err)
	}
	return nil
}

func (dh *dspHandlers) providerTransferRequestHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "providerTransferRequestHandler")
	defer span.End()
	ctx = ctxslog.With(ctx, "handler", "providerTransferRequestHandler")
	transferReq, err := shared.DecodeValid[shared.TransferRequestMessage](req)
	if err != nil {
		return transferError(ctx, fmt.Sprintf("invalid request message: %s", err.Error()),
			http.StatusBadRequest, "400", "Invalid request", nil)
	}

	consumerPID, err := uuid.Parse(transferReq.ConsumerPID)
	if err != nil {
		return transferError(ctx, fmt.Sprintf("Invalid consumer ID %s: %s", transferReq.ConsumerPID, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: ConsumerPID is not a UUID", nil)
	}
	agreementID, err := uuid.Parse(transferReq.AgreementID)
	if err != nil {
		return transferError(ctx, fmt.Sprintf("Invalid agreement ID %s: %s", transferReq.AgreementID, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: agreement ID is not a UUID", nil)
	}
	agreement, err := dh.store.GetAgreement(ctx, agreementopts.WithAgreementID(agreementID))
	if err != nil {
		return transferError(ctx, fmt.Sprintf("Could not get agreement with ID %s: %s", agreementID, err),
			http.StatusNotFound, "404", "Invalid request: Agreement not found", nil)
	}

	cbURL, err := url.Parse(transferReq.CallbackAddress)
	if err != nil {
		return transferError(ctx, fmt.Sprintf("Invalid callback URL %s: %s", transferReq.CallbackAddress, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: Non-valid callback URL.", nil)
	}

	var pi *dsrpc.PublishInfo

	if transferReq.DataAddress != nil {
		pi, err = statemachine.DataAddressToPublishInfo(transferReq.DataAddress)
		if err != nil {
			return transferError(ctx, fmt.Sprintf("Invalid callback URL %s: %s", transferReq.CallbackAddress, err.Error()),
				http.StatusBadRequest, "400", "Invalid request: Invalid dataAddress supplied.", nil)
		}
	}

	request := transfer.New(
		ctx,
		consumerPID,
		agreement,
		transferReq.Format,
		cbURL,
		dh.selfURL,
		constants.DataspaceProvider,
		transfer.States.INITIAL,
		pi,
		authforwarder.ExtractRequesterInfo(ctx),
	)

	if err := storeRequest(ctx, dh.store, request); err != nil {
		return err
	}

	req = req.WithContext(ctx)
	return processTransferMessage(dh, w, req, request.GetRole(), request.GetProviderPID(), true, transferReq)
}

func progressTransferState[T any](
	dh *dspHandlers, w http.ResponseWriter, req *http.Request, role constants.DataspaceRole,
	rawPID string, autoProgress bool,
) error {
	ctx, span := tracer.Start(req.Context(), "progressTransferState")
	defer span.End()
	pid, err := uuid.Parse(rawPID)
	if err != nil {
		return transferError(ctx, fmt.Sprintf("Invalid PID %s: %s", rawPID, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: PID is not a UUID", nil)
	}

	msg, err := shared.DecodeValid[T](req)
	if err != nil {
		return transferError(ctx, fmt.Sprintf("could not decode message: %s", err),
			http.StatusBadRequest, "400", "Invalid request", nil)
	}
	ctxslog.Debug(ctx, "Got contract message", "req", msg)
	return processTransferMessage(dh, w, req, role, pid, autoProgress, msg)
}

func processTransferMessage[T any](
	dh *dspHandlers,
	w http.ResponseWriter,
	req *http.Request,
	role constants.DataspaceRole,
	pid uuid.UUID,
	autoProgress bool,
	msg T,
) error {
	ctx, span := tracer.Start(req.Context(), "processTransferMessage")
	defer span.End()
	transfer, err := dh.store.GetTransfer(
		ctx,
		transferopts.WithRW(),
		transferopts.WithRolePID(pid, role),
	)
	if err != nil {
		return transferError(ctx, fmt.Sprintf("%d transfer request %s not found: %s", role, pid, err),
			http.StatusNotFound, "404", "Transfer request not found", nil)
	}
	ctx = ctxslog.With(ctx, transfer.GetLogFields("_recv")...)
	ctx = ctxslog.With(ctx, "messageType", fmt.Sprintf("%T", msg))
	ctxslog.Info(ctx, "processing transfer request")

	pState := statemachine.GetTransferRequestNegotiation(transfer, dh.provider, dh.reconciler)

	nextState, err := pState.Recv(ctx, msg)
	if err != nil {
		return transferError(ctx, fmt.Sprintf("invalid request: %s", err),
			http.StatusBadRequest, "400", "Invalid request", pState.GetTransferRequest())
	}

	apply := func() {}
	if autoProgress {
		apply, err = nextState.Send(ctx)
		if err != nil {
			return transferError(ctx, fmt.Sprintf("couldn't progress to next state: %s", err.Error()),
				http.StatusInternalServerError, "500", "Not able to progress state", nextState.GetTransferRequest())
		}
	}

	if err := storeRequest(ctx, dh.store, nextState.GetTransferRequest()); err != nil {
		return err
	}

	if err := shared.EncodeValid(w, req, http.StatusOK, nextState.GetTransferProcess()); err != nil {
		ctxslog.Err(ctx, "Couldn't serve response", err)
	}

	go apply()

	return nil
}

func storeRequest(
	ctx context.Context,
	store persistence.StorageProvider,
	request *transfer.Request,
) error {
	if err := store.PutTransfer(ctx, request); err != nil {
		return transferError(ctx, fmt.Sprintf("couldn't store transfer request: %s", err),
			http.StatusInternalServerError, "500", "Not able to store transfer request", request)
	}
	return nil
}

func (dh *dspHandlers) providerTransferStartHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "providerTransferStartHandler")
	defer span.End()
	req = req.WithContext(ctxslog.With(ctx, "handler", "providerTransferStartHandler"))
	return progressTransferState[shared.TransferStartMessage](
		dh, w, req, constants.DataspaceProvider, req.PathValue("providerPID"), false,
	)
}

func (dh *dspHandlers) providerTransferCompletionHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "providerTransferCompletionHandler")
	defer span.End()
	req = req.WithContext(ctxslog.With(ctx, "handler", "providerTransferCompletionHandler"))
	return progressTransferState[shared.TransferCompletionMessage](
		dh, w, req, constants.DataspaceProvider, req.PathValue("providerPID"), true,
	)
}

func (dh *dspHandlers) providerTransferTerminationHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "providerTransferTerminationHandler")
	defer span.End()
	req = req.WithContext(ctxslog.With(ctx, "handler", "providerTransferTerminationHandler"))
	return progressTransferState[shared.TransferTerminationMessage](
		dh, w, req, constants.DataspaceProvider, req.PathValue("providerPID"), true,
	)
}

// TODO: Handle suspension.
func (dh *dspHandlers) providerTransferSuspensionHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "providerTransferSuspensionHandler")
	defer span.End()
	providerPID := req.PathValue("providerPID")
	if providerPID == "" {
		return fmt.Errorf("missing provider PID")
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("could not read body")
	}
	suspension, err := shared.UnmarshalAndValidate(ctx, reqBody, shared.TransferSuspensionMessage{})
	if err != nil {
		return fmt.Errorf("invalid request")
	}

	ctxslog.Debug(ctx, "Got transfer suspension", "suspension", suspension)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)

	return nil
}

func (dh *dspHandlers) consumerTransferStartHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "consumerTransferStartHandler")
	defer span.End()
	req = req.WithContext(ctxslog.With(ctx, "handler", "consumerTransferStartHandler"))
	return progressTransferState[shared.TransferStartMessage](
		dh, w, req, constants.DataspaceConsumer, req.PathValue("consumerPID"), false,
	)
}

func (dh *dspHandlers) consumerTransferCompletionHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "consumerTransferCompletionHandler")
	defer span.End()
	req = req.WithContext(ctxslog.With(ctx, "handler", "consumerTransferCompletionHandler"))
	return progressTransferState[shared.TransferCompletionMessage](
		dh, w, req, constants.DataspaceConsumer, req.PathValue("consumerPID"), true,
	)
}

func (dh *dspHandlers) consumerTransferTerminationHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "consumerTransferTerminationHandler")
	defer span.End()
	req = req.WithContext(ctxslog.With(ctx, "handler", "consumerTransferTerminationHandler"))
	return progressTransferState[shared.TransferTerminationMessage](
		dh, w, req, constants.DataspaceConsumer, req.PathValue("consumerPID"), true,
	)
}

// TODO: Handle suspension.
func (dh *dspHandlers) consumerTransferSuspensionHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "consumerTransferSuspensionHandler")
	defer span.End()
	consumerPID := req.PathValue("providerPID")
	if consumerPID == "" {
		return fmt.Errorf("missing consumner PID")
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	suspension, err := shared.UnmarshalAndValidate(ctx, reqBody, shared.TransferSuspensionMessage{})
	if err != nil {
		return err
	}

	ctxslog.Debug(ctx, "Got transfer suspension", "suspension", suspension)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
	return nil
}
