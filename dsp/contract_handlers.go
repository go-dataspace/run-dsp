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

	"github.com/go-dataspace/run-dsp/dsp/constants"
	"github.com/go-dataspace/run-dsp/dsp/contract"
	"github.com/go-dataspace/run-dsp/dsp/persistence"
	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/go-dataspace/run-dsp/odrl"
	"github.com/google/uuid"
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
	err string, statusCode int, dspCode string, reason string, contract *contract.Negotiation,
) ContractError {
	return ContractError{
		status:   statusCode,
		contract: contract,
		dspCode:  dspCode,
		reason:   reason,
		err:      err,
	}
}

func (dh *dspHandlers) providerContractStateHandler(w http.ResponseWriter, req *http.Request) error {
	logger := logging.Extract(req.Context())
	providerPID, err := uuid.Parse(req.PathValue("providerPID"))
	if err != nil {
		return contractError("invalid provider ID", http.StatusBadRequest, "400", "Invalid provider PID", nil)
	}

	contract, err := dh.store.GetContractR(req.Context(), providerPID, constants.DataspaceProvider)
	if err != nil {
		return contractError(err.Error(), http.StatusNotFound, "404", "Contract not found", nil)
	}

	if err := shared.EncodeValid(w, req, http.StatusOK, contract.GetContractNegotiation()); err != nil {
		logger.Error("couldn't serve contract state", "err", err)
	}
	return nil
}

func (dh *dspHandlers) providerContractRequestHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, logger := logging.InjectLabels(req.Context(), "handler", "providerContractRequestHandler")
	req = req.WithContext(ctx)
	contractReq, err := shared.DecodeValid[shared.ContractRequestMessage](req)
	if err != nil {
		return contractError(fmt.Sprintf("invalid request message: %s", err.Error()),
			http.StatusBadRequest, "400", "Invalid request", nil)
	}
	logger.Debug("Got contract request", "req", contractReq)

	consumerPID, err := uuid.Parse(contractReq.ConsumerPID)
	if err != nil {
		return contractError(fmt.Sprintf("Invalid consumer ID %s: %s", contractReq.ConsumerPID, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: ConsumerPID is not a UUID", nil)
	}

	// TODO: Check if callback URL is reachable?
	cbURL, err := url.Parse(contractReq.CallbackAddress)
	if err != nil {
		return contractError(fmt.Sprintf("Invalid callback URL %s: %s", contractReq.CallbackAddress, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: Non-valid callback URL.", nil)
	}

	negotiation := contract.New(
		uuid.UUID{},
		consumerPID,
		contract.States.INITIAL,
		odrl.Offer{MessageOffer: contractReq.Offer},
		cbURL,
		dh.selfURL,
		constants.DataspaceProvider,
	)

	if err := storeNegotiation(ctx, dh.store, negotiation); err != nil {
		return err
	}

	return processMessage(dh, w, req, negotiation.GetRole(), negotiation.GetProviderPID(), contractReq)
}

func progressContractState[T any](
	dh *dspHandlers, w http.ResponseWriter, req *http.Request, role constants.DataspaceRole, rawPID string,
) error {
	logger := logging.Extract(req.Context())
	pid, err := uuid.Parse(rawPID)
	if err != nil {
		return contractError(fmt.Sprintf("Invalid PID %s: %s", rawPID, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: PID is not a UUID", nil)
	}
	msg, err := shared.DecodeValid[T](req)
	if err != nil {
		return contractError(fmt.Sprintf("could not decode message: %s", err),
			http.StatusBadRequest, "400", "Invalid request", nil)
	}

	logger.Debug("Got contract message", "req", msg)

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
	logger := logging.Extract(req.Context())
	contract, err := dh.store.GetContractRW(req.Context(), pid, role)
	if err != nil {
		return contractError(fmt.Sprintf("%d contract %s not found: %s", role, pid, err),
			http.StatusNotFound, "404", "Contract not found", nil)
	}

	ctx, pState := statemachine.GetContractNegotiation(
		req.Context(),
		contract,
		dh.provider,
		dh.reconciler,
	)

	ctx, nextState, err := pState.Recv(ctx, msg)
	if err != nil {
		return contractError(fmt.Sprintf("invalid request: %s", err),
			http.StatusBadRequest, "400", "Invalid request", pState.GetContract())
	}

	apply, err := nextState.Send(ctx)
	if err != nil {
		return contractError(fmt.Sprintf("couldn't progress to next state: %s", err.Error()),
			http.StatusInternalServerError, "500", "Not able to progress state", nextState.GetContract())
	}
	err = storeNegotiation(ctx, dh.store, nextState.GetContract())
	if err != nil {
		return err
	}

	if err := shared.EncodeValid(w, req, http.StatusOK, nextState.GetContractNegotiation()); err != nil {
		logger.Error("Couldn't serve response", "err", err)
	}

	go apply()
	return nil
}

func storeNegotiation(
	ctx context.Context,
	store persistence.StorageProvider,
	negotiation *contract.Negotiation,
) error {
	if err := store.PutContract(ctx, negotiation); err != nil {
		return contractError(fmt.Sprintf("couldn't store negotiation: %s", err),
			http.StatusInternalServerError, "500", "Not able to store negotiation", negotiation)
	}

	if negotiation.Modified() && negotiation.GetAgreement() != nil {
		if err := store.PutAgreement(ctx, negotiation.GetAgreement()); err != nil {
			return contractError(fmt.Sprintf("couldn't store agreement: %s", err),
				http.StatusInternalServerError, "500", "Not able to store agreement", negotiation)
		}
	}
	return nil
}

func (dh *dspHandlers) providerContractSpecificRequestHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, _ := logging.InjectLabels(req.Context(), "handler", "providerContractSpecificRequestHandler")
	req = req.WithContext(ctx)
	return progressContractState[shared.ContractRequestMessage](
		dh, w, req, constants.DataspaceProvider, req.PathValue("providerPID"),
	)
}

func (dh *dspHandlers) providerContractEventHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, _ := logging.InjectLabels(req.Context(), "handler", "providerContractEventHandler")
	req = req.WithContext(ctx)
	return progressContractState[shared.ContractNegotiationEventMessage](
		dh, w, req, constants.DataspaceProvider, req.PathValue("providerPID"),
	)
}

func (dh *dspHandlers) providerContractVerificationHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, _ := logging.InjectLabels(req.Context(), "handler", "providerContractVerificationHandler")
	req = req.WithContext(ctx)
	return progressContractState[shared.ContractAgreementVerificationMessage](
		dh, w, req, constants.DataspaceProvider, req.PathValue("providerPID"),
	)
}

func (dh *dspHandlers) providerContractTerminationHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, _ := logging.InjectLabels(req.Context(), "handler", "providerContractVerificationHandler")
	req = req.WithContext(ctx)
	return progressContractState[shared.ContractNegotiationTerminationMessage](
		dh, w, req, constants.DataspaceProvider, req.PathValue("providerPID"),
	)
}

func (dh *dspHandlers) consumerContractOfferHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, logger := logging.InjectLabels(req.Context(), "handler", "consumerContractOfferHandler")
	req = req.WithContext(ctx)
	contractOffer, err := shared.DecodeValid[shared.ContractOfferMessage](req)
	if err != nil {
		return contractError(fmt.Sprintf("invalid request message: %s", err.Error()),
			http.StatusBadRequest, "400", "Invalid request", nil)
	}
	logger.Debug("Got contract offer", "offer", contractOffer)

	providerPID, err := uuid.Parse(contractOffer.ProviderPID)
	if err != nil {
		return contractError(fmt.Sprintf("Invalid providerPID ID %s: %s", contractOffer.ProviderPID, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: ProviderPID is not a UUID", nil)
	}

	cbURL, err := url.Parse(contractOffer.CallbackAddress)
	if err != nil {
		return contractError(fmt.Sprintf("Invalid callback URL %s: %s", contractOffer.CallbackAddress, err.Error()),
			http.StatusBadRequest, "400", "Invalid request: Non-valid callback URL.", nil)
	}

	selfURL, err := url.Parse(dh.selfURL.String())
	if err != nil {
		panic(err.Error())
	}
	selfURL.Path = path.Join(selfURL.Path, "callback")

	negotiation := contract.New(
		providerPID,
		uuid.UUID{},
		contract.States.INITIAL,
		odrl.Offer{MessageOffer: contractOffer.Offer},
		cbURL,
		selfURL,
		constants.DataspaceConsumer,
	)
	if err := storeNegotiation(ctx, dh.store, negotiation); err != nil {
		return err
	}

	return processMessage(dh, w, req, negotiation.GetRole(), negotiation.GetConsumerPID(), contractOffer)
}

func (dh *dspHandlers) consumerContractSpecificOfferHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, _ := logging.InjectLabels(req.Context(), "handler", "consumerContractSpecificOfferHandler")
	req = req.WithContext(ctx)
	return progressContractState[shared.ContractOfferMessage](
		dh, w, req, constants.DataspaceConsumer, req.PathValue("consumerPID"),
	)
}

func (dh *dspHandlers) consumerContractAgreementHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, _ := logging.InjectLabels(req.Context(), "handler", "consumerContractAgreementHandler")
	req = req.WithContext(ctx)
	return progressContractState[shared.ContractAgreementMessage](
		dh, w, req, constants.DataspaceConsumer, req.PathValue("consumerPID"),
	)
}

func (dh *dspHandlers) consumerContractEventHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, _ := logging.InjectLabels(req.Context(), "handler", "consumerContractEventHandler")
	req = req.WithContext(ctx)
	return progressContractState[shared.ContractNegotiationEventMessage](
		dh, w, req, constants.DataspaceConsumer, req.PathValue("consumerPID"),
	)
}

func (dh *dspHandlers) consumerContractTerminationHandler(w http.ResponseWriter, req *http.Request) error {
	ctx, _ := logging.InjectLabels(req.Context(), "handler", "consumerContractEventHandler")
	req = req.WithContext(ctx)
	return progressContractState[shared.ContractNegotiationTerminationMessage](
		dh, w, req, constants.DataspaceConsumer, req.PathValue("consumerPID"),
	)
}
