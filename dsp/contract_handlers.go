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
	"net/http"
	"net/url"
	"path"
	"reflect"

	"github.com/google/uuid"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/contract"
	"go-dataspace.eu/run-dsp/dsp/persistence"
	"go-dataspace.eu/run-dsp/dsp/shared"
	"go-dataspace.eu/run-dsp/dsp/statemachine"
	"go-dataspace.eu/run-dsp/internal/authforwarder"
	"go-dataspace.eu/run-dsp/odrl"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

type ContractError struct {
	status   int
	contract *contract.Negotiation
	dspCode  string
	reason   string
	err      string
}

func (ce ContractError) Error() string     { return ce.err }
func (ce ContractError) StatusCode() int   { return ce.status }
func (ce ContractError) ErrorType() string { return "dspace:ContractNegotiationError" }
func (ce ContractError) DSPCode() string   { return ce.dspCode }

func (ce ContractError) Description() []shared.Multilanguage {
	return []shared.Multilanguage{{Value: ce.reason, Language: "en"}}
}

func (ce ContractError) Reason() []shared.Multilanguage {
	return []shared.Multilanguage{{Value: ce.reason, Language: "en"}}
}

func (ce ContractError) ProviderPID() string {
	if ce.contract == nil {
		return ""
	}
	return ce.contract.GetProviderPID().URN()
}

func (ce ContractError) ConsumerPID() string {
	if ce.contract == nil {
		return ""
	}
	return ce.contract.GetConsumerPID().URN()
}

func contractError(
	ctx context.Context, err string, statusCode int, dspCode string, reason string, contract *contract.Negotiation,
) ContractError {
	ctxslog.Error(
		ctx, "contract error",
		"statusCode", statusCode,
		"dspCode", dspCode,
		"reason", reason,
		"negotiationRole", contract.GetRole(),
		"localPID", contract.GetLocalPID(),
		"err", err,
	)
	return ContractError{
		status:   statusCode,
		contract: contract,
		dspCode:  dspCode,
		reason:   reason,
		err:      err,
	}
}

func (dh *dspHandlers) providerContractStateHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "providerContractStateHandler")
	defer span.End()
	providerPID, err := uuid.Parse(req.PathValue("providerPID"))
	if err != nil {
		return contractError(ctx, "invalid provider ID", http.StatusBadRequest, "400", "Invalid provider PID", nil)
	}

	contract, err := dh.store.GetContractR(ctx, providerPID, constants.DataspaceProvider)
	if err != nil {
		return contractError(ctx, err.Error(), http.StatusNotFound, "404", "Contract not found", nil)
	}

	if err := shared.EncodeValid(w, req, http.StatusOK, contract.GetContractNegotiation()); err != nil {
		ctxslog.Err(ctx, "couldn't serve contract state", err)
	}
	return nil
}

func (dh *dspHandlers) providerContractRequestHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "providerContractRequestHandler")
	defer span.End()
	ctx = ctxslog.With(ctx, "providerContractRequestHandler")
	contractReq, err := shared.DecodeValid[shared.ContractRequestMessage](req)
	if err != nil {
		return contractError(ctx, fmt.Sprintf("invalid request message: %s", err.Error()),
			http.StatusBadRequest, "400", "Invalid request", nil)
	}
	ctxslog.Debug(ctx, "Got contract request", contractReq)

	consumerPID, err := uuid.Parse(contractReq.ConsumerPID)
	if err != nil {
		return contractError(ctx, fmt.Sprintf("Invalid consumer ID %s: %s", contractReq.ConsumerPID, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: ConsumerPID is not a UUID", nil)
	}

	// TODO: Check if callback URL is reachable?
	cbURL, err := url.Parse(contractReq.CallbackAddress)
	if err != nil {
		return contractError(ctx, fmt.Sprintf("Invalid callback URL %s: %s", contractReq.CallbackAddress, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: Non-valid callback URL.", nil)
	}
	negotiation := contract.New(
		ctx,
		uuid.UUID{},
		consumerPID,
		contract.States.INITIAL,
		odrl.Offer{MessageOffer: contractReq.Offer},
		cbURL,
		dh.selfURL,
		constants.DataspaceProvider,
		dh.contractService == nil || reflect.ValueOf(dh.contractService).IsNil(),
		authforwarder.ExtractRequesterInfo(ctx),
	)

	if err := storeNegotiation(ctx, dh.store, negotiation); err != nil {
		return err
	}

	return processMessage(dh, w, req, negotiation.GetRole(), negotiation.GetProviderPID(), contractReq)
}

func progressContractState[T any](
	dh *dspHandlers, w http.ResponseWriter, req *http.Request, role constants.DataspaceRole, rawPID string,
) error {
	ctx, span := tracer.Start(req.Context(), "progressContractState")
	defer span.End()
	pid, err := uuid.Parse(rawPID)
	if err != nil {
		return contractError(ctx, fmt.Sprintf("Invalid PID %s: %s", rawPID, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: PID is not a UUID", nil)
	}
	msg, err := shared.DecodeValid[T](req)
	if err != nil {
		return contractError(ctx, fmt.Sprintf("could not decode message: %s", err),
			http.StatusBadRequest, "400", "Invalid request", nil)
	}

	ctxslog.Debug(ctx, "Got contract message", "req", msg)

	// Since this is a continuation of negotiation, we add a continuation RequestInfo to context
	// FIXME: This probably has to be reconsidered regarding security.
	requestInfo := &dsrpc.RequesterInfo{
		AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_CONTINUATION,
	}

	// Add local origin requester info to context
	req = req.WithContext(authforwarder.SetRequesterInfo(ctx, requestInfo))

	return processMessage(dh, w, req, role, pid, msg)
}

func processMessage[T any](
	dh *dspHandlers,
	w http.ResponseWriter,
	req *http.Request,
	role constants.DataspaceRole,
	pid uuid.UUID,
	msg T,
) error {
	ctx, span := tracer.Start(req.Context(), "processMessage")
	defer span.End()
	contract, err := dh.store.GetContractRW(ctx, pid, role)
	if err != nil {
		return contractError(ctx, fmt.Sprintf("%d contract %s not found: %s", role, pid, err),
			http.StatusNotFound, "404", "Contract not found", nil)
	}
	ctx = ctxslog.With(ctx, contract.GetLogFields("_recv")...)
	ctx = ctxslog.With(ctx, "messageType", fmt.Sprintf("%T", msg))
	ctxslog.Info(ctx, "processing contract negotiation")
	pState := statemachine.GetContractNegotiation(
		contract,
		dh.provider,
		dh.contractService,
		dh.reconciler,
	)

	ctx, apply, err := pState.Recv(ctx, msg)
	if err != nil {
		return contractError(ctx, fmt.Sprintf("invalid request: %s", err),
			http.StatusBadRequest, "400", "Invalid request", pState.GetContract())
	}

	if contract.AutoAccept() || (dh.contractService == nil || reflect.ValueOf(dh.contractService).IsNil()) {
		ctx = ctxslog.With(ctx, pState.GetContract().GetLogFields("_send")...)
		transition := statemachine.GetContractNegotiation(
			pState.GetContract(),
			dh.provider,
			dh.contractService,
			dh.reconciler,
		)
		apply, err = transition.Send(ctx)
		if err != nil {
			return contractError(ctx, fmt.Sprintf("failed to send: %s", err),
				http.StatusInternalServerError, "500", "Internal error", pState.GetContract(),
			)
		}
	}

	err = storeNegotiation(ctx, dh.store, pState.GetContract())
	if err != nil {
		return err
	}
	if err := apply(); err != nil {
		return contractError(ctx, fmt.Sprintf("failed to propagate: %s", err),
			http.StatusInternalServerError, "500", "Internal error", pState.GetContract(),
		)
	}
	if err := shared.EncodeValid(w, req, http.StatusOK, pState.GetContractNegotiation()); err != nil {
		ctxslog.Err(ctx, "Couldn't serve response", err)
	}

	return nil
}

func storeNegotiation(
	ctx context.Context,
	store persistence.StorageProvider,
	negotiation *contract.Negotiation,
) error {
	if err := store.PutContract(ctx, negotiation); err != nil {
		return contractError(ctx, fmt.Sprintf("couldn't store negotiation: %s", err),
			http.StatusInternalServerError, "500", "Not able to store negotiation", negotiation)
	}

	if negotiation.Modified() && negotiation.GetAgreement() != nil {
		if err := store.PutAgreement(ctx, negotiation.GetAgreement()); err != nil {
			return contractError(ctx, fmt.Sprintf("couldn't store agreement: %s", err),
				http.StatusInternalServerError, "500", "Not able to store agreement", negotiation)
		}
	}
	return nil
}

func (dh *dspHandlers) providerContractSpecificRequestHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "providerContractSpecificRequestHandler")
	defer span.End()
	ctx = ctxslog.With(ctx, "handler", "providerContractSpecificRequestHandler")
	req = req.WithContext(ctx)
	return progressContractState[shared.ContractRequestMessage](
		dh, w, req, constants.DataspaceProvider, req.PathValue("providerPID"),
	)
}

func (dh *dspHandlers) providerContractEventHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "providerContractEventHandler")
	defer span.End()
	ctx = ctxslog.With(ctx, "handler", "providerContractEventHandler")
	req = req.WithContext(ctx)
	return progressContractState[shared.ContractNegotiationEventMessage](
		dh, w, req, constants.DataspaceProvider, req.PathValue("providerPID"),
	)
}

func (dh *dspHandlers) providerContractVerificationHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "providerContractVerificationHandler")
	defer span.End()
	ctx = ctxslog.With(ctx, "handler", "providerContractVerificationHandler")
	req = req.WithContext(ctx)
	return progressContractState[shared.ContractAgreementVerificationMessage](
		dh, w, req, constants.DataspaceProvider, req.PathValue("providerPID"),
	)
}

func (dh *dspHandlers) contractTerminationHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "contractTerminationHandler")
	defer span.End()
	ctx = ctxslog.With(ctx, "handler", "providerContractVerificationHandler")
	req = req.WithContext(ctx)
	pid := req.PathValue("PID")
	id, err := uuid.Parse(pid)
	if err != nil {
		return contractError(ctx, fmt.Sprintf("Invalid PID: %s", pid),
			http.StatusBadRequest, "400", "Invalid request: PID is not a UUID", nil)
	}
	if _, err := dh.store.GetContractR(ctx, id, constants.DataspaceProvider); err == nil {
		return progressContractState[shared.ContractNegotiationTerminationMessage](
			dh, w, req, constants.DataspaceProvider, pid,
		)
	}
	return progressContractState[shared.ContractNegotiationTerminationMessage](
		dh, w, req, constants.DataspaceConsumer, pid,
	)
}

func (dh *dspHandlers) consumerContractOfferHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "consumerContractOfferHandler")
	defer span.End()
	ctx = ctxslog.With(ctx, "handler", "consumerContractOfferHandler")
	req = req.WithContext(ctx)
	contractOffer, err := shared.DecodeValid[shared.ContractOfferMessage](req)
	if err != nil {
		return contractError(ctx, fmt.Sprintf("invalid request message: %s", err.Error()),
			http.StatusBadRequest, "400", "Invalid request", nil)
	}
	ctxslog.Debug(ctx, "Got contract offer", "offer", contractOffer)

	providerPID, err := uuid.Parse(contractOffer.ProviderPID)
	if err != nil {
		return contractError(ctx, fmt.Sprintf("Invalid providerPID ID %s: %s", contractOffer.ProviderPID, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: ProviderPID is not a UUID", nil)
	}

	cbURL, err := url.Parse(contractOffer.CallbackAddress)
	if err != nil {
		return contractError(ctx, fmt.Sprintf("Invalid callback URL %s: %s", contractOffer.CallbackAddress, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: Non-valid callback URL.", nil)
	}

	selfURL, err := url.Parse(dh.selfURL.String())
	if err != nil {
		panic(err.Error())
	}
	selfURL.Path = path.Join(selfURL.Path, "callback")

	negotiation := contract.New(
		ctx,
		providerPID,
		uuid.New(),
		contract.States.INITIAL,
		odrl.Offer{MessageOffer: contractOffer.Offer},
		cbURL,
		selfURL,
		constants.DataspaceConsumer,
		dh.contractService == nil || reflect.ValueOf(dh.contractService).IsNil(),
		authforwarder.ExtractRequesterInfo(ctx),
	)
	if err := storeNegotiation(ctx, dh.store, negotiation); err != nil {
		return err
	}

	return processMessage(dh, w, req, negotiation.GetRole(), negotiation.GetConsumerPID(), contractOffer)
}

func (dh *dspHandlers) consumerContractSpecificOfferHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "consumerContractSpecificOfferHandler")
	defer span.End()
	ctx = ctxslog.With(ctx, "handler", "consumerContractSpecificOfferHandler")
	req = req.WithContext(ctx)
	return progressContractState[shared.ContractOfferMessage](
		dh, w, req, constants.DataspaceConsumer, req.PathValue("consumerPID"),
	)
}

func (dh *dspHandlers) consumerContractAgreementHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "consumerContractAgreementHandler")
	defer span.End()
	ctx = ctxslog.With(ctx, "handler", "consumerContractAgreementHandler")
	req = req.WithContext(ctx)
	return progressContractState[shared.ContractAgreementMessage](
		dh, w, req, constants.DataspaceConsumer, req.PathValue("consumerPID"),
	)
}

func (dh *dspHandlers) consumerContractEventHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, span := tracer.Start(req.Context(), "consumerContractEventHandler")
	defer span.End()
	ctx = ctxslog.With(ctx, "handler", "consumerContractEventHandler")
	req = req.WithContext(ctx)
	return progressContractState[shared.ContractNegotiationEventMessage](
		dh, w, req, constants.DataspaceConsumer, req.PathValue("consumerPID"),
	)
}
